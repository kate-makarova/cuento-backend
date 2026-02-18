package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var jwtKey = []byte("your_secret_key") // In production, use environment variable

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Claims struct {
	Username string `json:"username"`
	UserID   int    `json:"user_id"`
	jwt.RegisteredClaims
}

func Register(c *gin.Context, db *sql.DB) {
	var user Entities.User
	if err := c.ShouldBindJSON(&user); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	if err := user.HashPassword(user.Password); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to hash password"})
		c.Abort()
		return
	}

	query := "INSERT INTO users (username, email, password, date_registered) VALUES (?, ?, ?, ?)"
	res, err := db.Exec(query, user.Username, user.Email, user.Password, time.Now())
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to create user"})
		c.Abort()
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get user Id"})
		c.Abort()
		return
	}
	user.Id = int(id)

	// Get default role ID (assuming role with name "user" exists)
	var defaultRoleID int
	err = db.QueryRow("SELECT id FROM roles WHERE name = ?", "user").Scan(&defaultRoleID)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get default role"})
		c.Abort()
		return
	}

	// Assign default role to user
	_, err = db.Exec("INSERT INTO user_role (user_id, role_id) VALUES (?, ?)", user.Id, defaultRoleID)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to assign role to user"})
		c.Abort()
		return
	}

	user.Password = "" // Don't return password
	user.Roles = []Entities.Role{{Id: defaultRoleID, Name: "user"}}

	c.JSON(http.StatusCreated, user)
}

func Login(c *gin.Context, db *sql.DB) {
	var creds Credentials
	if err := c.ShouldBindJSON(&creds); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	var user Entities.User
	query := "SELECT id, username, avatar, email, password FROM users WHERE username = ?"
	err := db.QueryRow(query, creds.Username).Scan(&user.Id, &user.Username, &user.Avatar, &user.Email, &user.Password)
	if err != nil {
		if err == sql.ErrNoRows {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Invalid credentials"})
		} else {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Database error"})
		}
		c.Abort()
		return
	}

	// Fetch user roles from many-to-many relationship
	rolesQuery := `
		SELECT r.id, r.name
		FROM roles r
		INNER JOIN user_role ur ON r.id = ur.role_id
		WHERE ur.user_id = ?`
	rows, err := db.Query(rolesQuery, user.Id)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch user roles: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	user.Roles = []Entities.Role{}
	for rows.Next() {
		var role Entities.Role
		if err := rows.Scan(&role.Id, &role.Name); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan role: " + err.Error()})
			c.Abort()
			return
		}
		user.Roles = append(user.Roles, role)
	}

	// Check for errors during iteration
	if err := rows.Err(); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Error iterating roles: " + err.Error()})
		c.Abort()
		return
	}

	if err := user.CheckPassword(creds.Password); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Invalid credentials: " + err.Error()})
		c.Abort()
		return
	}

	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		Username: user.Username,
		UserID:   user.Id,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			Issuer:    "cuento-backend",
		},
	}

	user.Password = "" // Don't return password

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to generate token"})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": tokenString, "user": user})
}
