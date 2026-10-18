package Services

import "database/sql"

type ImageUploadResult struct {
	URL          string
	ThumbnailURL string
	DeleteURL    string
}

type BulkUploadFileResult struct {
	FileName string
	Result   *ImageUploadResult
	Err      error
}

type ImageClient interface {
	UploadImage(imageData []byte, db *sql.DB) (*ImageUploadResult, error)
	BulkUploadSupport() bool
}

// BulkImageClient is implemented by clients that support uploading an entire
// folder of images. Callers should check BulkUploadSupport() or type-assert
// to this interface before using it.
type BulkImageClient interface {
	ImageClient
	BulkUpload(folderPath string, db *sql.DB) ([]BulkUploadFileResult, error)
}
