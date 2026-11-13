package Services

import (
	"database/sql"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// superuserID is read once from the environment at startup.
var superuserID = func() int {
	if v := os.Getenv("SUPERUSER_ID"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			return id
		}
	}
	return 0
}()

// IsSuperuser returns true if the currently authenticated user is the superuser.
func IsSuperuser(c *gin.Context) bool {
	return superuserID != 0 && GetUserIdFromContext(c) == superuserID
}

// IsSuperuserID returns true if the given user ID is the superuser.
func IsSuperuserID(id int) bool {
	return superuserID != 0 && id == superuserID
}

// UserLocale holds timezone and language for a request's viewer, fetched once per handler.
type UserLocale struct {
	TZ   *string
	Lang string
}

// GetUserLocale fetches interface_timezone and interface_language for a user in one query.
// Returns nil timezone and "en" language for guests (userID == 0) or on error.
func GetUserLocale(userID int, db *sql.DB) UserLocale {
	locale := UserLocale{Lang: "en"}
	if userID == 0 {
		return locale
	}
	var tzNull, langNull sql.NullString
	if err := db.QueryRow("SELECT interface_timezone, interface_language FROM users WHERE id = ?", userID).Scan(&tzNull, &langNull); err != nil {
		return locale
	}
	if tzNull.Valid && tzNull.String != "" {
		locale.TZ = &tzNull.String
	}
	if langNull.Valid && langNull.String != "" {
		locale.Lang = langNull.String
	}
	return locale
}

// GetUserTimezone fetches only the interface_timezone for a given user.
// Use GetUserLocale when you also need the language for LocalizeTime.
func GetUserTimezone(userID int, db *sql.DB) *string {
	return GetUserLocale(userID, db).TZ
}

// GetUserLocaleFromContext retrieves the UserLocale stored by UserLocaleMiddleware.
// Falls back to the default locale when the middleware did not run (e.g. public routes).
func GetUserLocaleFromContext(c *gin.Context) UserLocale {
	if v, exists := c.Get("userLocale"); exists {
		if locale, ok := v.(UserLocale); ok {
			return locale
		}
	}
	return UserLocale{Lang: "en"}
}

// LocalizeTime converts t into the viewer's timezone, then formats it using locale-specific
// date format, AM/PM preference, and today/yesterday word substitution.
func LocalizeTime(t time.Time, locale UserLocale) string {
	loc := time.UTC
	if locale.TZ != nil && *locale.TZ != "" {
		if l, err := time.LoadLocation(*locale.TZ); err == nil {
			loc = l
		}
	}
	local := t.In(loc)

	localizer := NewLocalizer(locale.Lang)
	dateFormat := localeStr(localizer, "date.format", "2006-01-02")
	usePm := localeStr(localizer, "date.usePm", "false") == "true"
	todayWord := localeStr(localizer, "date.today", "")
	yesterdayWord := localeStr(localizer, "date.yesterday", "")

	timeFormat := "15:04:05"
	if usePm {
		timeFormat = "3:04:05 PM"
	}

	now := time.Now().In(loc)
	var datePart string
	switch {
	case todayWord != "" && sameDay(local, now):
		datePart = todayWord
	case yesterdayWord != "" && sameDay(local, now.AddDate(0, 0, -1)):
		datePart = yesterdayWord
	default:
		datePart = local.Format(dateFormat)
	}

	return datePart + " " + local.Format(timeFormat)
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func localeStr(localizer *i18n.Localizer, key, def string) string {
	msg, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
	if err != nil || msg == "" {
		return def
	}
	return msg
}

// LocalizeDateOnly formats a date-only value as "YYYY-MM-DD" with no timezone conversion.
// Use this for DATE columns (absence dates, expiration dates) where no time component exists.
func LocalizeDateOnly(t time.Time) string {
	return t.Format("2006-01-02")
}

// IsFeatureEnabled checks whether a named feature flag is active in the Gin context,
// as populated by FeatureFlagsMiddleware.
func IsFeatureEnabled(flagName string, c *gin.Context) bool {
	flagMap, _ := c.Get("features")
	flags, _ := flagMap.(map[string]bool)
	return flags[flagName]
}

// GetUserIdFromContext safely retrieves the user ID from the Gin context.
// It returns the user ID if it exists and is a valid integer.
// It returns 0 if the user ID does not exist or is not a valid type,
// indicating a guest user or an unauthenticated session.
func GetUserIdFromContext(c *gin.Context) int {
	if id, exists := c.Get("user_id"); exists {
		if userID, ok := id.(int); ok {
			return userID
		}
	}
	return 0
}
