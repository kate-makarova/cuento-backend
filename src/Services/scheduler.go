package Services

import (
	"database/sql"
	"log"
	"time"
)

type ScheduledJob struct {
	Name string
	Run  func(db *sql.DB)
}

// StartScheduler runs each job once on startup, then re-runs all of them every hour.
// Each job's Run function is responsible for its own "is it due?" check.
func StartScheduler(db *sql.DB, jobs []ScheduledJob) {
	go func() {
		for _, j := range jobs {
			runJob(j, db)
		}
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			for _, j := range jobs {
				runJob(j, db)
			}
		}
	}()
}

func runJob(j ScheduledJob, db *sql.DB) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("scheduler: job %q panicked: %v", j.Name, r)
		}
	}()
	j.Run(db)
}
