package Services

import (
	"context"
	"cuento-backend/config"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/qdrant/go-client/qdrant"
)

const embeddingDims = 1536 // text-embedding-3-small

var qdrantClient *qdrant.Client
var openaiEmbedClient openai.Client

func InitQdrant() {
	cfg := config.LoadQdrantConfig()
	if cfg.OpenAIKey == "" {
		log.Printf("Warning: OPENAI_API_KEY not set — Qdrant vector search will be unavailable")
		return
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   cfg.Host,
		Port:   cfg.Port,
		APIKey: cfg.APIKey,
	})
	if err != nil {
		log.Printf("Warning: could not connect to Qdrant at %s:%d: %v", cfg.Host, cfg.Port, err)
		return
	}

	ctx := context.Background()
	for _, bucket := range AllSonicBuckets {
		exists, err := client.CollectionExists(ctx, bucket)
		if err != nil {
			log.Printf("Warning: could not check Qdrant collection %s: %v", bucket, err)
			continue
		}
		if !exists {
			if err := client.CreateCollection(ctx, &qdrant.CreateCollection{
				CollectionName: bucket,
				VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
					Size:     embeddingDims,
					Distance: qdrant.Distance_Cosine,
				}),
			}); err != nil {
				log.Printf("Warning: could not create Qdrant collection %s: %v", bucket, err)
				continue
			}
		}
	}

	qdrantClient = client
	openaiEmbedClient = openai.NewClient(option.WithAPIKey(cfg.OpenAIKey))
	log.Printf("Qdrant initialized at %s:%d", cfg.Host, cfg.Port)
}

func QdrantAvailable() bool {
	return qdrantClient != nil
}

// GetQdrantCollectionCounts returns the number of indexed vectors per bucket.
func GetQdrantCollectionCounts() map[string]uint64 {
	counts := make(map[string]uint64, len(AllSonicBuckets))
	for _, bucket := range AllSonicBuckets {
		info, err := qdrantClient.GetCollectionInfo(context.Background(), bucket)
		if err != nil {
			counts[bucket] = 0
			continue
		}
		counts[bucket] = info.GetPointsCount()
	}
	return counts
}

func embedText(ctx context.Context, text string) ([]float32, error) {
	resp, err := openaiEmbedClient.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: openai.EmbeddingModelTextEmbedding3Small,
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: []string{text}},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	raw := resp.Data[0].Embedding
	vec := make([]float32, len(raw))
	for i, v := range raw {
		vec[i] = float32(v)
	}
	return vec, nil
}

func QdrantPush(bucket, id, text string) error {
	numID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}
	ctx := context.Background()
	vec, err := embedText(ctx, text)
	if err != nil {
		return fmt.Errorf("embedding failed for %s/%s: %w", bucket, id, err)
	}
	_, err = qdrantClient.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: bucket,
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewIDNum(numID),
				Vectors: qdrant.NewVectorsDense(vec),
			},
		},
	})
	return err
}

func QdrantDelete(bucket, id string) error {
	numID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}
	_, err = qdrantClient.Delete(context.Background(), &qdrant.DeletePoints{
		CollectionName: bucket,
		Points:         qdrant.NewPointsSelector(qdrant.NewIDNum(numID)),
	})
	return err
}

func QdrantDeleteBatch(bucket string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	pointIDs := make([]*qdrant.PointId, 0, len(ids))
	for _, id := range ids {
		numID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			continue
		}
		pointIDs = append(pointIDs, qdrant.NewIDNum(numID))
	}
	if len(pointIDs) == 0 {
		return nil
	}
	_, err := qdrantClient.Delete(context.Background(), &qdrant.DeletePoints{
		CollectionName: bucket,
		Points:         qdrant.NewPointsSelectorIDs(pointIDs),
	})
	return err
}

func QdrantQuery(bucket, query string, limit int) ([]string, error) {
	ctx := context.Background()
	vec, err := embedText(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding query failed: %w", err)
	}
	results, err := qdrantClient.Query(ctx, &qdrant.QueryPoints{
		CollectionName: bucket,
		Query:          qdrant.NewQueryDense(vec),
		Limit:          qdrant.PtrOf(uint64(limit)),
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, strconv.FormatUint(r.Id.GetNum(), 10))
	}
	return ids, nil
}

func QdrantPushFlattenedEntity(bucket, entityName string, id int64, db *sql.DB) error {
	rows, err := db.Query(
		fmt.Sprintf(`SELECT b.*, f.* FROM %s_base b LEFT JOIN %s_flattened f ON b.id = f.entity_id WHERE b.id = ?`, entityName, entityName),
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to query %s %d: %w", entityName, id, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("failed to read columns for %s: %w", entityName, err)
	}

	if !rows.Next() {
		return nil
	}

	entityID, doc, err := scanFlattenedRowForSonic(rows, cols)
	if err != nil || doc == "" {
		return err
	}

	return QdrantPush(bucket, strconv.FormatInt(entityID, 10), doc)
}

// QdrantSearchInBucket performs a semantic search via Qdrant, then applies the same
// permission and filter logic as SearchInBucket before fetching content from the DB.
func QdrantSearchInBucket(bucket, query string, limit, filterSubforum, filterTopicType int, visibleSubforums map[int]bool, db *sql.DB) ([]SearchResultItem, error) {
	qdrantIDs, err := QdrantQuery(bucket, query, limit)
	if err != nil || len(qdrantIDs) == 0 {
		return nil, nil
	}

	placeholders := strings.Repeat("?,", len(qdrantIDs)-1) + "?"
	args := make([]interface{}, len(qdrantIDs))
	for i, id := range qdrantIDs {
		args[i] = id
	}

	var metaQuery string
	switch bucket {
	case SonicBucketGamePosts, SonicBucketGeneralPosts, SonicBucketLorePosts:
		metaQuery = fmt.Sprintf(`SELECT p.id, t.subforum_id, t.type FROM posts p JOIN topics t ON p.topic_id = t.id WHERE p.id IN (%s)`, placeholders)
	case SonicBucketCharacters:
		metaQuery = fmt.Sprintf(`SELECT cb.id, t.subforum_id, t.type FROM character_base cb JOIN topics t ON cb.topic_id = t.id WHERE cb.id IN (%s)`, placeholders)
	case SonicBucketWantedPosts:
		metaQuery = fmt.Sprintf(`SELECT wc.id, t.subforum_id, t.type FROM wanted_character_base wc JOIN topics t ON wc.topic_id = t.id WHERE wc.id IN (%s)`, placeholders)
	case SonicBucketEpisodes:
		metaQuery = fmt.Sprintf(`SELECT eb.id, t.subforum_id, t.type FROM episode_base eb JOIN topics t ON eb.topic_id = t.id WHERE eb.id IN (%s)`, placeholders)
	default:
		return nil, fmt.Errorf("unknown bucket: %s", bucket)
	}

	metaRows, err := db.Query(metaQuery, args...)
	if err != nil {
		return nil, err
	}
	defer metaRows.Close()

	allowedIDs := make(map[string]bool)
	for metaRows.Next() {
		var rawID int64
		var subforumID, topicType int
		if err := metaRows.Scan(&rawID, &subforumID, &topicType); err != nil {
			continue
		}
		if !visibleSubforums[subforumID] {
			continue
		}
		if filterSubforum != 0 && subforumID != filterSubforum {
			continue
		}
		if filterTopicType != 0 && topicType != filterTopicType {
			continue
		}
		allowedIDs[strconv.FormatInt(rawID, 10)] = true
	}

	if len(allowedIDs) == 0 {
		return nil, nil
	}

	// Preserve Qdrant similarity ranking order.
	filtered := make([]string, 0, len(allowedIDs))
	for _, id := range qdrantIDs {
		if allowedIDs[id] {
			filtered = append(filtered, id)
		}
	}

	filteredPlaceholders := strings.Repeat("?,", len(filtered)-1) + "?"
	filteredArgs := make([]interface{}, len(filtered))
	for i, id := range filtered {
		filteredArgs[i] = id
	}

	switch bucket {
	case SonicBucketGamePosts, SonicBucketGeneralPosts, SonicBucketLorePosts:
		rows, err := db.Query(fmt.Sprintf(
			`SELECT p.id, p.content, p.topic_id, t.name, t.type
			 FROM posts p JOIN topics t ON p.topic_id = t.id
			 WHERE p.id IN (%s)`, filteredPlaceholders), filteredArgs...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		contentMap := make(map[string]SearchResultItem, len(filtered))
		for rows.Next() {
			var id, topicID int64
			var content, topicName string
			var topicType int
			if err := rows.Scan(&id, &content, &topicID, &topicName, &topicType); err != nil {
				continue
			}
			sid := strconv.FormatInt(id, 10)
			contentMap[sid] = SearchResultItem{
				ID:        sid,
				Content:   extractMatchingParagraphs(content, query),
				TopicID:   &topicID,
				TopicName: topicName,
				TopicType: &topicType,
			}
		}

		results := make([]SearchResultItem, 0, len(filtered))
		for _, id := range filtered {
			if item, ok := contentMap[id]; ok {
				results = append(results, item)
			}
		}
		return results, nil

	default:
		entityName := EntityBuckets[bucket]
		rows, err := db.Query(
			fmt.Sprintf(`SELECT b.*, f.*, t.name AS _topic_name, t.type AS _topic_type
			 FROM %s_base b
			 LEFT JOIN %s_flattened f ON b.id = f.entity_id
			 JOIN topics t ON b.topic_id = t.id
			 WHERE b.id IN (%s)`, entityName, entityName, filteredPlaceholders),
			filteredArgs...,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return nil, err
		}

		contentMap := make(map[string]SearchResultItem, len(filtered))
		for rows.Next() {
			id, topicID, topicName, topicType, doc, err := scanFlattenedRowWithTopicID(rows, cols)
			if err != nil {
				continue
			}
			sid := strconv.FormatInt(id, 10)
			contentMap[sid] = SearchResultItem{
				ID:        sid,
				Content:   extractMatchingParagraphs(doc, query),
				TopicID:   &topicID,
				TopicName: topicName,
				TopicType: &topicType,
			}
		}

		results := make([]SearchResultItem, 0, len(filtered))
		for _, id := range filtered {
			if item, ok := contentMap[id]; ok {
				results = append(results, item)
			}
		}
		return results, nil
	}
}
