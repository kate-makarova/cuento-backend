package test

import (
	"bytes"
	"cuento-backend/src/Controllers"
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestGetTopic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer db.Close()

	t.Run("Successful GetTopic", func(t *testing.T) {
		topicID := 1
		subforumID := 10
		now := time.Now()

		mock.ExpectQuery("SELECT t.id, t.status, t.name, t.type, t.date_created, t.date_last_post, t.post_number, t.author_user_id, u.username, t.last_post_author_user_id, u2.username, t.subforum_id FROM topics t").
			WithArgs(topicID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "status", "name", "type", "date_created", "date_last_post", "post_number", "author_user_id", "username", "last_post_author_user_id", "username2", "subforum_id"}).
				AddRow(topicID, 0, "Test Topic", 0, now, now, 1, 1, "author", 1, "author", subforumID))

		// Mock CanEdit check
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM role_permission").
			WithArgs(0, "subforum_edit_others_topic:10").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		r := gin.New()
		r.Use(Middlewares.ErrorMiddleware())
		r.GET("/topic/:id", func(c *gin.Context) {
			Controllers.GetTopic(c, db)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/topic/1", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var response Entities.Topic
		json.Unmarshal(w.Body.Bytes(), &response)
		if response.Name != "Test Topic" {
			t.Errorf("Expected topic name 'Test Topic', got %s", response.Name)
		}
	})
}

func TestCreatePost(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer db.Close()

	t.Run("Successful CreatePost", func(t *testing.T) {
		reqBody := Controllers.CreatePostRequest{
			TopicID: 1,
			Content: "New post content",
		}
		body, _ := json.Marshal(reqBody)

		mock.ExpectBegin()
		// The query has 5 placeholders. NOW() is not a placeholder.
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO posts (topic_id, author_user_id, content, date_created, use_character_profile, character_profile_id) VALUES (?, ?, ?, NOW(), ?, ?)")).
			WithArgs(1, 1, "New post content", false, nil).
			WillReturnResult(sqlmock.NewResult(100, 1))
		mock.ExpectCommit()

		// Mock Mentions check (none in this case)
		// Mock Subforum/Topic fetch for event
		mock.ExpectQuery("SELECT subforum_id, name FROM topics WHERE id = ?").
			WithArgs(1).
			WillReturnRows(sqlmock.NewRows([]string{"subforum_id", "name"}).AddRow(10, "Topic Name"))

		// Mock GetPostById for event
		mock.ExpectQuery("SELECT config FROM custom_field_config").WillReturnRows(sqlmock.NewRows([]string{"config"}).AddRow("[]"))
		mock.ExpectQuery("SELECT p.id, p.topic_id, p.author_user_id").WillReturnRows(sqlmock.NewRows([]string{"id", "topic_id", "author_user_id", "date_created", "content", "use_character_profile", "username", "avatar", "character_profile_id", "character_id", "character_name", "character_avatar"}).
			AddRow(100, 1, 1, "2023-01-01 00:00:00", "New post content", false, "user1", nil, nil, nil, nil, nil))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user_id", 1)
			c.Next()
		})
		r.Use(Middlewares.ErrorMiddleware())
		r.POST("/post", func(c *gin.Context) {
			Controllers.CreatePost(c, db)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/post", bytes.NewBuffer(body))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

func TestUpdatePost(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Successful UpdatePost", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()

		postID := 100
		reqBody := Controllers.UpdatePostRequest{
			Content: "Updated content",
		}
		body, _ := json.Marshal(reqBody)

		// 1. Fetch post details
		mock.ExpectQuery("SELECT p.author_user_id, t.subforum_id FROM posts p").
			WithArgs(postID).
			WillReturnRows(sqlmock.NewRows([]string{"author_user_id", "subforum_id"}).AddRow(1, 10))

		// 2. Check permissions (Edit own post)
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM role_permission").
			WithArgs(1, "subforum_edit_own_post:10").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		// 3. Update post
		mock.ExpectExec(regexp.QuoteMeta("UPDATE posts SET content = ? WHERE id = ?")).
			WithArgs("Updated content", postID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// 4. Fetch updated post for event
		mock.ExpectQuery("SELECT config FROM custom_field_config").WillReturnRows(sqlmock.NewRows([]string{"config"}).AddRow("[]"))
		mock.ExpectQuery("SELECT p.id, p.topic_id, p.author_user_id").WillReturnRows(sqlmock.NewRows([]string{"id", "topic_id", "author_user_id", "date_created", "content", "use_character_profile", "username", "avatar", "character_profile_id", "character_id", "character_name", "character_avatar"}).
			AddRow(postID, 1, 1, "2023-01-01 00:00:00", "Updated content", false, "user1", nil, nil, nil, nil, nil))

		// 5. Fetch topicID for event
		mock.ExpectQuery("SELECT topic_id FROM posts WHERE id = ?").
			WithArgs(postID).
			WillReturnRows(sqlmock.NewRows([]string{"topic_id"}).AddRow(1))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user_id", 1)
			c.Next()
		})
		r.Use(Middlewares.ErrorMiddleware())
		r.POST("/post/update/:id", func(c *gin.Context) {
			Controllers.UpdatePost(c, db)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/post/update/100", bytes.NewBuffer(body))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

func TestUpdateTopic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Successful UpdateTopic", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()

		topicID := 1
		name := "Updated Topic Name"
		reqBody := Controllers.UpdateTopicRequest{
			Name: &name,
		}
		body, _ := json.Marshal(reqBody)

		// 1. Fetch topic details
		mock.ExpectQuery("SELECT author_user_id, subforum_id, type FROM topics WHERE id = ?").
			WithArgs(topicID).
			WillReturnRows(sqlmock.NewRows([]string{"author_user_id", "subforum_id", "type"}).AddRow(1, 10, 0)) // 0 = GeneralTopic

		// 2. Check permissions (Edit own topic)
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM role_permission").
			WithArgs(1, "subforum_edit_own_topic:10").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		// 3. Update topic
		mock.ExpectExec(regexp.QuoteMeta("UPDATE topics SET name = ? WHERE id = ?")).
			WithArgs("Updated Topic Name", topicID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user_id", 1)
			c.Next()
		})
		r.Use(Middlewares.ErrorMiddleware())
		r.POST("/topic/update/:id", func(c *gin.Context) {
			Controllers.UpdateTopic(c, db)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/topic/update/1", bytes.NewBuffer(body))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Update Non-General Topic", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()

		topicID := 2
		name := "Updated Topic Name"
		reqBody := Controllers.UpdateTopicRequest{
			Name: &name,
		}
		body, _ := json.Marshal(reqBody)

		// 1. Fetch topic details (EpisodeTopic = 1)
		mock.ExpectQuery("SELECT author_user_id, subforum_id, type FROM topics WHERE id = ?").
			WithArgs(topicID).
			WillReturnRows(sqlmock.NewRows([]string{"author_user_id", "subforum_id", "type"}).AddRow(1, 10, 1))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user_id", 1)
			c.Next()
		})
		r.Use(Middlewares.ErrorMiddleware())
		r.POST("/topic/update/:id", func(c *gin.Context) {
			Controllers.UpdateTopic(c, db)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/topic/update/2", bytes.NewBuffer(body))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}
