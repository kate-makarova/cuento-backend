package Services

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Events"
	"database/sql"
	"log"
	"time"
)

func runAbsenceStartedNotifications(db *sql.DB) {
	rows, err := db.Query(`
		SELECT au.id, au.user_id, au.absence_end_date
		FROM absent_users au
		WHERE DATE(au.absence_start_date) = CURDATE()
		  AND au.is_deleted = 0
		  AND au.start_notified = 0`,
	)
	if err != nil {
		log.Printf("Absence started notifications: failed to query: %v", err)
		return
	}
	defer rows.Close()

	type absenceRow struct {
		id             int
		userID         int
		absenceEndDate time.Time
	}
	var absences []absenceRow
	for rows.Next() {
		var a absenceRow
		if rows.Scan(&a.id, &a.userID, &a.absenceEndDate) == nil {
			absences = append(absences, a)
		}
	}

	for _, a := range absences {
		if _, err := db.Exec("UPDATE absent_users SET start_notified = 1 WHERE id = ?", a.id); err != nil {
			log.Printf("Absence started notifications: failed to mark notified for absence %d: %v", a.id, err)
			continue
		}
		Events.Publish(db, Events.UserAbsenceStarted, Events.UserAbsenceStartedEvent{
			UserID:         a.userID,
			AbsenceEndDate: a.absenceEndDate,
		})
	}
}

var archivingWarningThresholds = []int{10, 5, 3, 2, 1}

func StartArchivingNotifier(db *sql.DB) {
	go func() {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.UTC)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}

		time.Sleep(time.Until(next))
		runAutoArchiving(db)
		runArchivingNotifications(db)
		runAbsenceStartedNotifications(db)

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			runAutoArchiving(db)
			runArchivingNotifications(db)
			runAbsenceStartedNotifications(db)
		}
	}()
}

func runAutoArchiving(db *sql.DB) {
	if !GetAutoArchivingEnabled(db) {
		return
	}

	autoArchivingDays := GetAutoArchivingDays(db)

	rows, err := db.Query(`
		SELECT cb.id, cb.name, cb.user_id
		FROM character_base cb
		JOIN users u ON u.id = cb.user_id
		JOIN absence_timer_start ats ON ats.character_id = cb.id
		LEFT JOIN absent_users au ON au.user_id = cb.user_id
			AND au.absence_start_date <= NOW() AND au.absence_end_date >= NOW() AND au.is_deleted = 0
		LEFT JOIN auto_archiving_immunity aai ON aai.character_id = cb.id
			AND aai.start_date <= NOW() AND aai.end_date >= NOW()
		WHERE cb.character_status = ?
		AND u.user_status = ?
		AND DATEDIFF(NOW(), ats.start_date) - ats.extra_days >= ?
		AND au.id IS NULL
		AND aai.id IS NULL
	`, Entities.ActiveCharacter, Entities.ActiveUser, autoArchivingDays)
	if err != nil {
		log.Printf("Auto archiving: failed to query characters: %v", err)
		return
	}

	type charRow struct {
		id         int
		name       string
		userID     int
		topicID    int
		subforumID int
		topicStatus int
	}

	var chars []charRow
	for rows.Next() {
		var ch charRow
		if err := rows.Scan(&ch.id, &ch.name, &ch.userID); err != nil {
			continue
		}
		_ = db.QueryRow(
			"SELECT t.id, t.subforum_id, t.status FROM topics t JOIN character_base cb ON cb.topic_id = t.id WHERE cb.id = ?",
			ch.id,
		).Scan(&ch.topicID, &ch.subforumID, &ch.topicStatus)
		chars = append(chars, ch)
	}
	rows.Close()

	for _, ch := range chars {
		tx, err := db.Begin()
		if err != nil {
			log.Printf("Auto archiving: failed to begin transaction for character %d: %v", ch.id, err)
			continue
		}

		if _, err := tx.Exec(
			"UPDATE character_base SET character_status = ? WHERE id = ?",
			Entities.InactiveCharacter, ch.id,
		); err != nil {
			log.Printf("Auto archiving: failed to deactivate character %d: %v", ch.id, err)
			tx.Rollback()
			continue
		}

		if ch.topicID > 0 {
			if _, err := tx.Exec("UPDATE topics SET status = ? WHERE id = ?", Entities.InactiveTopic, ch.topicID); err != nil {
				log.Printf("Auto archiving: failed to deactivate topic %d for character %d: %v", ch.topicID, ch.id, err)
				tx.Rollback()
				continue
			}
		}

		if _, err := tx.Exec("UPDATE character_profile_base SET is_archived = 1 WHERE character_id = ?", ch.id); err != nil {
			log.Printf("Auto archiving: failed to archive profiles for character %d: %v", ch.id, err)
			tx.Rollback()
			continue
		}

		var claimID int
		if err := tx.QueryRow("SELECT claim_id FROM claim_record WHERE character_id = ? ORDER BY claim_date DESC LIMIT 1", ch.id).Scan(&claimID); err == nil {
			_, _ = tx.Exec("UPDATE character_claim SET is_claimed = false WHERE id = ?", claimID)
			_, _ = tx.Exec("UPDATE wanted_character_base SET is_claimed = false WHERE character_claim_id = ?", claimID)
		}

		if err := tx.Commit(); err != nil {
			log.Printf("Auto archiving: failed to commit for character %d: %v", ch.id, err)
			continue
		}

		Events.Publish(db, Events.CharacterStatusChanged, Events.CharacterStatusChangedEvent{
			CharacterID: ch.id,
			UserID:      ch.userID,
			OldStatus:   Entities.ActiveCharacter,
			NewStatus:   Entities.InactiveCharacter,
		})
		Events.Publish(db, Events.CharacterDeactivated, Events.CharacterDeactivatedEvent{CharacterID: ch.id})
		if ch.topicID > 0 && ch.topicStatus != int(Entities.InactiveTopic) {
			Events.Publish(db, Events.TopicStatusChanged, Events.TopicStatusChangedEvent{
				TopicID:    int64(ch.topicID),
				SubforumID: ch.subforumID,
				OldStatus:  ch.topicStatus,
				NewStatus:  int(Entities.InactiveTopic),
			})
		}

		lang := GetUserLanguage(ch.userID, db)
		localizer := NewLocalizer(lang)
		Events.Publish(db, Events.NotificationCreated, Events.NotificationEvent{
			UserID: ch.userID,
			Type:   "auto_archiving",
			Message: TData(localizer, "auto_archiving.deactivated", map[string]interface{}{
				"Name": ch.name,
			}),
			Data: map[string]interface{}{
				"character_id":   ch.id,
				"character_name": ch.name,
			},
		})
	}
}

func runArchivingNotifications(db *sql.DB) {
	if !GetAutoArchivingEnabled(db) {
		return
	}

	autoArchivingDays := GetAutoArchivingDays(db)

	// Warn immune characters whose protection expires at a threshold day and will be immediately archived.
	for _, threshold := range archivingWarningThresholds {
		rows, err := db.Query(`
			SELECT
				cb.id,
				cb.name,
				cb.user_id,
				DATE(aai_exp.end_date) AS base_date
			FROM character_base cb
			JOIN users u ON u.id = cb.user_id
			JOIN absence_timer_start ats ON ats.character_id = cb.id
			JOIN (
				SELECT character_id, MAX(end_date) AS end_date
				FROM auto_archiving_immunity
				WHERE end_date >= NOW()
				GROUP BY character_id
			) aai_exp ON aai_exp.character_id = cb.id
			LEFT JOIN absent_users au ON au.user_id = cb.user_id
				AND au.absence_start_date <= NOW() AND au.absence_end_date >= NOW() AND au.is_deleted = 0
			WHERE cb.character_status = ?
			AND u.user_status = ?
			AND DATEDIFF(aai_exp.end_date, NOW()) = ?
			AND ? - DATEDIFF(aai_exp.end_date, ats.start_date) <= 0
			AND au.id IS NULL
		`, Entities.ActiveCharacter, Entities.ActiveUser, threshold, autoArchivingDays)
		if err != nil {
			log.Printf("Archiving notifier (immunity expiry): failed to query for threshold %d: %v", threshold, err)
			continue
		}

		type charRow struct {
			id       int
			name     string
			userID   int
			baseDate string
		}

		var chars []charRow
		for rows.Next() {
			var ch charRow
			if err := rows.Scan(&ch.id, &ch.name, &ch.userID, &ch.baseDate); err != nil {
				continue
			}
			chars = append(chars, ch)
		}
		rows.Close()

		for _, ch := range chars {
			var count int
			if err := db.QueryRow(
				"SELECT COUNT(*) FROM archiving_warning_log WHERE character_id = ? AND days_warning = ? AND base_date = ?",
				ch.id, threshold, ch.baseDate,
			).Scan(&count); err != nil || count > 0 {
				continue
			}

			lang := GetUserLanguage(ch.userID, db)
			localizer := NewLocalizer(lang)
			Events.Publish(db, Events.NotificationCreated, Events.NotificationEvent{
				UserID: ch.userID,
				Type:   "auto_archiving",
				Message: TPlural(localizer, "auto_archiving.immunity_expiry_warning", threshold, map[string]interface{}{
					"Name":  ch.name,
					"Count": threshold,
				}),
				Data: map[string]interface{}{
					"character_id":   ch.id,
					"character_name": ch.name,
					"days_left":      threshold,
				},
			})

			if _, err := db.Exec(
				"INSERT INTO archiving_warning_log (character_id, days_warning, base_date) VALUES (?, ?, ?)",
				ch.id, threshold, ch.baseDate,
			); err != nil {
				log.Printf("Archiving notifier (immunity expiry): failed to log notification for character %d: %v", ch.id, err)
			}
		}
	}

	for _, threshold := range archivingWarningThresholds {
		rows, err := db.Query(`
			SELECT
				cb.id,
				cb.name,
				cb.user_id,
				ats.start_date AS base_date
			FROM character_base cb
			JOIN users u ON u.id = cb.user_id
			JOIN absence_timer_start ats ON ats.character_id = cb.id
			LEFT JOIN absent_users au ON au.user_id = cb.user_id
				AND au.absence_start_date <= NOW() AND au.absence_end_date >= NOW() AND au.is_deleted = 0
			LEFT JOIN auto_archiving_immunity aai ON aai.character_id = cb.id
				AND aai.start_date <= NOW() AND aai.end_date >= NOW()
			WHERE cb.character_status = ?
			AND u.user_status = ?
			AND ? - (DATEDIFF(NOW(), ats.start_date) - ats.extra_days) = ?
			AND au.id IS NULL
			AND aai.id IS NULL
		`, Entities.ActiveCharacter, Entities.ActiveUser, autoArchivingDays, threshold)
		if err != nil {
			log.Printf("Archiving notifier: failed to query characters for threshold %d: %v", threshold, err)
			continue
		}

		type charRow struct {
			id       int
			name     string
			userID   int
			baseDate string
		}

		var chars []charRow
		for rows.Next() {
			var ch charRow
			if err := rows.Scan(&ch.id, &ch.name, &ch.userID, &ch.baseDate); err != nil {
				continue
			}
			chars = append(chars, ch)
		}
		rows.Close()

		for _, ch := range chars {
			var count int
			if err := db.QueryRow(
				"SELECT COUNT(*) FROM archiving_warning_log WHERE character_id = ? AND days_warning = ? AND base_date = ?",
				ch.id, threshold, ch.baseDate,
			).Scan(&count); err != nil || count > 0 {
				continue
			}

			lang := GetUserLanguage(ch.userID, db)
			localizer := NewLocalizer(lang)
			Events.Publish(db, Events.NotificationCreated, Events.NotificationEvent{
				UserID: ch.userID,
				Type:   "auto_archiving",
				Message: TPlural(localizer, "auto_archiving.archiving_warning", threshold, map[string]interface{}{
					"Name":  ch.name,
					"Count": threshold,
				}),
				Data: map[string]interface{}{
					"character_id":   ch.id,
					"character_name": ch.name,
					"days_left":      threshold,
				},
			})

			if _, err := db.Exec(
				"INSERT INTO archiving_warning_log (character_id, days_warning, base_date) VALUES (?, ?, ?)",
				ch.id, threshold, ch.baseDate,
			); err != nil {
				log.Printf("Archiving notifier: failed to log notification for character %d: %v", ch.id, err)
			}
		}
	}
}
