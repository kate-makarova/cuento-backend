package Services

import "database/sql"

// GetImageClient returns the configured image upload client.
// Falls back to ImgbbClient if the setting is missing or unrecognised.
func GetImageClient(db *sql.DB) BulkImageClient {
	provider, _ := GetGlobalSetting("image_upload_provider", db)
	return getImageClientFromValue(provider)
}

func getImageClientFromValue(provider string) BulkImageClient {
	if provider == "local" {
		return LocalImageClient{}
	}
	return ImgbbClient{}
}

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

// BulkImageClient is implemented by clients that can upload a folder of images.
// BulkUploadSupport() indicates whether the provider has a native batch API;
// if false, uploads are performed one at a time.
// onProgress is called after each file with (completed, total) counts so the
// caller can stream progress to the frontend.
type BulkImageClient interface {
	ImageClient
	BulkUpload(folderPath string, db *sql.DB, onProgress func(completed, total int)) ([]BulkUploadFileResult, error)
}
