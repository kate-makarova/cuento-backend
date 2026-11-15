package test

import (
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

func TestGetUnreadNotifications(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer db.Close()

	t.Run("Successful GetUnreadNotifications", func(t *testing.T) {
		userID := 1
		now := time.Now()

		mock.ExpectQuery("SELECT id, type, title, message, data, date_created, is_read FROM notifications").
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "type", "title", "message", "data", "date_created", "is_read"}).
				AddRow(1, "mention", "Title", "Message", `{"user_id": 2, "user_name": "user2", "post_id": 10, "topic_id": 100}`, now, false))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
		r.Use(Middlewares.ErrorMiddleware())
		r.GET("/notifications/unread", func(c *gin.Context) {
			Controllers.GetUnreadNotifications(c, db)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/notifications/unread", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var response map[string][]Entities.MentionNotification
		json.Unmarshal(w.Body.Bytes(), &response)
		if len(response["mention"]) != 1 {
			t.Errorf("Expected 1 mention notification, got %d", len(response["mention"]))
		}
		if response["mention"][0].Data.UserName != "user2" {
			t.Errorf("Expected mention author 'user2', got %s", response["mention"][0].Data.UserName)
		}
	})
}

func TestDismissNotification(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}
	defer db.Close()

	t.Run("Successful DismissNotification", func(t *testing.T) {
		userID := 1
		notificationID := 100

		mock.ExpectExec(regexp.QuoteMeta("UPDATE notifications SET is_read = TRUE WHERE id = ? AND user_id = ?")).
			WithArgs(notificationID, userID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
			c.Next()
		})
		r.Use(Middlewares.ErrorMiddleware())
		r.POST("/notifications/dismiss/:id", func(c *gin.Context) {
			Controllers.DismissNotification(c, db)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/notifications/dismiss/100", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}
