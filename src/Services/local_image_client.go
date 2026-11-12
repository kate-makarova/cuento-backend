package Services

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const staticImagesDir = "/home/static-images"

var localImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".webp": true, ".bmp": true,
}

type LocalImageClient struct{}

func (LocalImageClient) BulkUploadSupport() bool { return false }

func (c LocalImageClient) UploadImage(imageData []byte, db *sql.DB) (*ImageUploadResult, error) {
	baseURL, err := GetGlobalSetting("image_hosting_base_url", db)
	if err != nil || strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("image_hosting_base_url is not configured")
	}
	return localClientUploadToDir(imageData, baseURL, staticImagesDir)
}

func localClientUploadToDir(imageData []byte, baseURL string, dir string) (*ImageUploadResult, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("image_hosting_base_url is not configured")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	ext := detectImageExt(imageData)
	filename := uuid.New().String() + ext
	dest := filepath.Join(dir, filename)

	if err := os.WriteFile(dest, imageData, 0644); err != nil {
		return nil, fmt.Errorf("writing image to %s: %w", dest, err)
	}

	url := baseURL + "/" + filename
	return &ImageUploadResult{
		URL:          url,
		ThumbnailURL: url,
		DeleteURL:    "",
	}, nil
}

func (c LocalImageClient) BulkUpload(folderPath string, db *sql.DB, onProgress func(completed, total int)) ([]BulkUploadFileResult, error) {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read folder: %w", err)
	}

	var files []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && localImageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			files = append(files, e)
		}
	}

	total := len(files)
	results := make([]BulkUploadFileResult, total)

	for i, e := range files {
		data, err := os.ReadFile(filepath.Join(folderPath, e.Name()))
		if err != nil {
			results[i] = BulkUploadFileResult{FileName: e.Name(), Err: err}
		} else {
			res, err := c.UploadImage(data, db)
			results[i] = BulkUploadFileResult{FileName: e.Name(), Result: res, Err: err}
		}
		if onProgress != nil {
			onProgress(i+1, total)
		}
	}

	return results, nil
}

// detectImageExt sniffs the first bytes to determine the image format.
// Falls back to .jpg if unrecognised.
func detectImageExt(data []byte) string {
	if len(data) < 4 {
		return ".jpg"
	}
	switch {
	case data[0] == 0x89 && data[1] == 0x50: // PNG
		return ".png"
	case data[0] == 0xFF && data[1] == 0xD8: // JPEG
		return ".jpg"
	case data[0] == 0x47 && data[1] == 0x49: // GIF
		return ".gif"
	case data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46: // RIFF → WebP
		return ".webp"
	case data[0] == 0x42 && data[1] == 0x4D: // BMP
		return ".bmp"
	default:
		return ".jpg"
	}
}
