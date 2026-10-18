package Services

import "database/sql"

type ImageUploadResult struct {
	URL          string
	ThumbnailURL string
	DeleteURL    string
}

type ImageClient interface {
	UploadImage(imageData []byte, db *sql.DB) (*ImageUploadResult, error)
	BulkUploadSupport() bool
}
