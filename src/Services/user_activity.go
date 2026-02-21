package Services

import (
	"sync"
	"time"
)

type UserActivity struct {
	UserID          int       `json:"user_id"`
	Username        string    `json:"username"`
	CurrentPageType string    `json:"current_page_type"`
	CurrentPageId   string    `json:"current_page_id"`
	LastActive      time.Time `json:"last_active"`
}

type UserActivityStorage struct {
	users map[int]*UserActivity
	mu    sync.RWMutex
}

var ActivityStorage = UserActivityStorage{
	users: make(map[int]*UserActivity),
}

func (s *UserActivityStorage) AddUser(userID int, username string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[userID]; !exists {
		s.users[userID] = &UserActivity{
			UserID:     userID,
			Username:   username,
			LastActive: time.Now(),
		}
	} else {
		// Update existing user's last active time
		s.users[userID].LastActive = time.Now()
	}
}

func (s *UserActivityStorage) RemoveUser(userID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, userID)
}

func (s *UserActivityStorage) UpdateUserLocation(userID int, pageType string, pageId string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if user, exists := s.users[userID]; exists {
		user.CurrentPageType = pageType
		user.CurrentPageId = pageId
		user.LastActive = time.Now()
	}
}

func (s *UserActivityStorage) GetActiveUsers() []*UserActivity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	activeUsers := make([]*UserActivity, 0, len(s.users))
	for _, user := range s.users {
		activeUsers = append(activeUsers, user)
	}
	return activeUsers
}

func (s *UserActivityStorage) GetUsersOnPage(pageType string, pageId string) []*UserActivity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	usersOnPage := make([]*UserActivity, 0)
	for _, user := range s.users {
		if user.CurrentPageType == pageType && user.CurrentPageId == pageId {
			usersOnPage = append(usersOnPage, user)
		}
	}
	return usersOnPage
}
