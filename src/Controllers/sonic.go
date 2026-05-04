package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/expectedsh/go-sonic/sonic"
	"github.com/gin-gonic/gin"
)

type SonicCursorItem struct {
	Bucket       string     `json:"bucket"`
	LastId       *int64     `json:"last_id"`
	DateIngested *time.Time `json:"date_ingested"`
	CurrentMaxId int64      `json:"current_max_id"`
}

var allBuckets = []string{
	Services.SonicBucketGamePosts,
	Services.SonicBucketGeneralPosts,
	Services.SonicBucketLorePosts,
	Services.SonicBucketCharacters,
	Services.SonicBucketWantedPosts,
	Services.SonicBucketEpisodes,
}

// entityBuckets maps bucket name to the entity base table name for flattened ingestion.
var entityBuckets = map[string]string{
	Services.SonicBucketCharacters:  "character",
	Services.SonicBucketWantedPosts: "wanted_character",
	Services.SonicBucketEpisodes:    "episode",
}

func GetSonicCursors(c *gin.Context, db *sql.DB) {
	rows, err := db.Query("SELECT bucket, last_id, date_ingested FROM sonic_ingest_cursor")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch cursors: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	cursorMap := make(map[string]*SonicCursorItem)
	for rows.Next() {
		var item SonicCursorItem
		if err := rows.Scan(&item.Bucket, &item.LastId, &item.DateIngested); err == nil {
			cursorMap[item.Bucket] = &item
		}
	}

	result := make([]SonicCursorItem, 0, len(allBuckets))
	for _, bucket := range allBuckets {
		item := SonicCursorItem{Bucket: bucket}
		if existing, ok := cursorMap[bucket]; ok {
			item.LastId = existing.LastId
			item.DateIngested = existing.DateIngested
		}
		item.CurrentMaxId = getBucketMaxId(bucket, db)
		result = append(result, item)
	}

	c.JSON(http.StatusOK, result)
}

func CatchUpSonicBucket(c *gin.Context, db *sql.DB) {
	bucket := c.Param("bucket")

	if !isKnownBucket(bucket) {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Unknown bucket: " + bucket})
		c.Abort()
		return
	}

	var fromId int64
	_ = db.QueryRow("SELECT last_id FROM sonic_ingest_cursor WHERE bucket = ?", bucket).Scan(&fromId)

	var lastId int64
	var count int
	var ingestErr error

	if entityName, isEntity := entityBuckets[bucket]; isEntity {
		lastId, count, ingestErr = ingestFlattenedBucket(bucket, entityName, fromId, db)
	} else {
		lastId, count, ingestErr = ingestPostBucket(bucket, fromId, db)
	}

	if ingestErr != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: ingestErr.Error()})
		c.Abort()
		return
	}

	if count > 0 {
		_, err := db.Exec(
			`INSERT INTO sonic_ingest_cursor (bucket, last_id, date_ingested) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE last_id = VALUES(last_id), date_ingested = VALUES(date_ingested)`,
			bucket, lastId, time.Now(),
		)
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update cursor: " + err.Error()})
			c.Abort()
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"bucket":   bucket,
		"ingested": count,
		"last_id":  lastId,
	})
}

// ingestPostBucket handles posts buckets: queries (id, content) and pushes each post.
func ingestPostBucket(bucket string, fromId int64, db *sql.DB) (lastId int64, count int, err error) {
	rows, err := fetchPostRows(bucket, fromId, db)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch posts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			continue
		}
		if err := Services.SonicPush(Services.SonicCollection, bucket, strconv.FormatInt(id, 10), text, sonic.LangAutoDetect); err != nil {
			return lastId, count, fmt.Errorf("failed to push post %d to Sonic: %w", id, err)
		}
		if id > lastId {
			lastId = id
		}
		count++
	}
	return lastId, count, nil
}

// ingestFlattenedBucket handles entity buckets: reads all columns dynamically from the
// base+flattened join and builds a text document from every non-null value.
func ingestFlattenedBucket(bucket, entityName string, fromId int64, db *sql.DB) (lastId int64, count int, err error) {
	query := fmt.Sprintf(
		`SELECT b.*, f.* FROM %s_base b
		 LEFT JOIN %s_flattened f ON b.id = f.entity_id
		 WHERE b.id > ? ORDER BY b.id ASC`,
		entityName, entityName,
	)
	rows, err := db.Query(query, fromId)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch %s entities: %w", entityName, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read columns: %w", err)
	}

	for rows.Next() {
		id, doc, err := scanFlattenedRow(rows, cols)
		if err != nil {
			continue
		}
		if doc == "" {
			continue
		}
		if err := Services.SonicPush(Services.SonicCollection, bucket, strconv.FormatInt(id, 10), doc, sonic.LangAutoDetect); err != nil {
			return lastId, count, fmt.Errorf("failed to push %s %d to Sonic: %w", entityName, id, err)
		}
		if id > lastId {
			lastId = id
		}
		count++
	}
	return lastId, count, nil
}

// scanFlattenedRow scans a row into a map and returns the entity id and a space-joined
// text document built from all non-null column values (entity_id is skipped as a duplicate of id).
func scanFlattenedRow(rows *sql.Rows, cols []string) (int64, string, error) {
	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return 0, "", err
	}

	var id int64
	var parts []string

	for i, col := range cols {
		raw := vals[i]
		if raw == nil {
			continue
		}

		// Convert value to string
		var s string
		switch v := raw.(type) {
		case []byte:
			s = string(v)
		case string:
			s = v
		case int64:
			s = strconv.FormatInt(v, 10)
		default:
			s = fmt.Sprintf("%v", v)
		}

		if col == "id" {
			// Parse entity id from the base table's first id column
			if id == 0 {
				id, _ = strconv.ParseInt(s, 10, 64)
			}
			continue
		}
		if col == "entity_id" {
			// Duplicate of id from the flattened table — skip
			continue
		}

		s = strings.TrimSpace(s)
		if s != "" {
			parts = append(parts, s)
		}
	}

	return id, strings.Join(parts, " "), nil
}

func fetchPostRows(bucket string, fromId int64, db *sql.DB) (*sql.Rows, error) {
	topicType, ok := postBucketTopicType(bucket)
	if !ok {
		return nil, fmt.Errorf("unknown post bucket: %s", bucket)
	}
	return db.Query(
		`SELECT p.id, p.content FROM posts p
		 JOIN topics t ON p.topic_id = t.id
		 WHERE t.type = ? AND COALESCE(p.is_deleted, 0) != 1 AND p.id > ?
		 ORDER BY p.id ASC`,
		topicType, fromId,
	)
}

func postBucketTopicType(bucket string) (Entities.TopicType, bool) {
	switch bucket {
	case Services.SonicBucketGamePosts:
		return Entities.EpisodeTopic, true
	case Services.SonicBucketGeneralPosts:
		return Entities.GeneralTopic, true
	case Services.SonicBucketLorePosts:
		return Entities.LoreTopic, true
	}
	return 0, false
}

func getBucketMaxId(bucket string, db *sql.DB) int64 {
	var maxId int64
	switch bucket {
	case Services.SonicBucketGamePosts:
		_ = db.QueryRow(`SELECT COALESCE(MAX(p.id), 0) FROM posts p JOIN topics t ON p.topic_id = t.id WHERE t.type = ? AND COALESCE(p.is_deleted, 0) != 1`, Entities.EpisodeTopic).Scan(&maxId)
	case Services.SonicBucketGeneralPosts:
		_ = db.QueryRow(`SELECT COALESCE(MAX(p.id), 0) FROM posts p JOIN topics t ON p.topic_id = t.id WHERE t.type = ? AND COALESCE(p.is_deleted, 0) != 1`, Entities.GeneralTopic).Scan(&maxId)
	case Services.SonicBucketLorePosts:
		_ = db.QueryRow(`SELECT COALESCE(MAX(p.id), 0) FROM posts p JOIN topics t ON p.topic_id = t.id WHERE t.type = ? AND COALESCE(p.is_deleted, 0) != 1`, Entities.LoreTopic).Scan(&maxId)
	case Services.SonicBucketCharacters:
		_ = db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM character_base`).Scan(&maxId)
	case Services.SonicBucketWantedPosts:
		_ = db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM wanted_character_base`).Scan(&maxId)
	case Services.SonicBucketEpisodes:
		_ = db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM episode_base`).Scan(&maxId)
	}
	return maxId
}

func isKnownBucket(bucket string) bool {
	for _, b := range allBuckets {
		if b == bucket {
			return true
		}
	}
	return false
}
