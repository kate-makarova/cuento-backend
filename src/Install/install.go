package Install

import (
	"database/sql"
	"os"
	"regexp"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func ExecuteSQLFile(db *sql.DB, filePath string) error {
	// 1. Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// 2. Use regex to split by one or more empty lines
	// This matches \n\n, \r\n\r\n, or even multiple empty lines in a row
	re := regexp.MustCompile(`(?m)^\s*$\s*`)
	statements := re.Split(string(content), -1)

	// 3. Execute each statement separately
	for _, statement := range statements {
		query := strings.TrimSpace(statement)

		// Skip if the segment is empty (e.g., trailing newlines at end of file)
		if query == "" {
			continue
		}

		_, err := db.Exec(query)
		if err != nil {
			return err
		}
	}

	return nil
}
