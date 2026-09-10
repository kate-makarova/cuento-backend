package Services

import (
	"database/sql"
	"fmt"

	webpush "github.com/SherClockHolmes/webpush-go"
)

const (
	vapidPrivateKeySetting = "vapid_private_key"
	vapidPublicKeySetting  = "vapid_public_key"
	vapidSubject           = "mailto:admin@cuento.app"
)

func GetOrCreateVAPIDKeys(db *sql.DB) (public, private string, err error) {
	public, _ = GetGlobalSetting(vapidPublicKeySetting, db)
	private, _ = GetGlobalSetting(vapidPrivateKeySetting, db)
	if public != "" && private != "" {
		return public, private, nil
	}

	private, public, err = webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", fmt.Errorf("generate VAPID keys: %w", err)
	}

	_, err = db.Exec(
		`INSERT INTO global_settings (setting_name, setting_value) VALUES (?, ?), (?, ?)
		 ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)`,
		vapidPublicKeySetting, public, vapidPrivateKeySetting, private,
	)
	if err != nil {
		return "", "", fmt.Errorf("save VAPID keys: %w", err)
	}
	return public, private, nil
}

func SendPushToUser(db *sql.DB, userID int, notificationType, title, message string) {
	privateKey, err := GetGlobalSetting(vapidPrivateKeySetting, db)
	if err != nil || privateKey == "" {
		return
	}
	publicKey, err := GetGlobalSetting(vapidPublicKeySetting, db)
	if err != nil || publicKey == "" {
		return
	}

	rows, err := db.Query(
		"SELECT endpoint, p256dh, auth FROM user_push_subscriptions WHERE user_id = ?", userID,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	payload := fmt.Sprintf(`{"type":%q,"title":%q,"message":%q}`, notificationType, title, message)

	for rows.Next() {
		var endpoint, p256dh, auth string
		if err := rows.Scan(&endpoint, &p256dh, &auth); err != nil {
			continue
		}
		sub := &webpush.Subscription{
			Endpoint: endpoint,
			Keys: webpush.Keys{
				P256dh: p256dh,
				Auth:   auth,
			},
		}
		resp, err := webpush.SendNotification([]byte(payload), sub, &webpush.Options{
			VAPIDPublicKey:  publicKey,
			VAPIDPrivateKey: privateKey,
			Subscriber:      vapidSubject,
			TTL:             86400,
		})
		if err != nil {
			fmt.Printf("push send error for user %d: %v\n", userID, err)
			continue
		}
		resp.Body.Close()
		// Remove expired/invalid subscriptions (410 Gone, 404 Not Found).
		if resp.StatusCode == 410 || resp.StatusCode == 404 {
			db.Exec(
				"DELETE FROM user_push_subscriptions WHERE user_id = ? AND endpoint = ?",
				userID, endpoint,
			)
		}
	}
}
