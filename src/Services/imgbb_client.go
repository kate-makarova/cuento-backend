package Services

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

var imgbbImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".webp": true, ".bmp": true,
}

type ImgbbClient struct{}

func (ImgbbClient) BulkUploadSupport() bool { return true }

func (c ImgbbClient) UploadImage(imageData []byte, db *sql.DB) (*ImageUploadResult, error) {
	return UploadImageToImgbb(imageData, db)
}

func (c ImgbbClient) BulkUpload(folderPath string, db *sql.DB) ([]BulkUploadFileResult, error) {
	apiKey, err := GetGlobalSetting("imgbb_api_key", db)
	if err != nil || apiKey == "" {
		return nil, fmt.Errorf("imgbb_api_key is not configured")
	}

	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read folder: %w", err)
	}

	var fileNames []string
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for _, e := range entries {
		if e.IsDir() || !imgbbImageExts[filepath.Ext(e.Name())] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(folderPath, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", e.Name(), err)
		}
		part, err := writer.CreateFormFile("image[]", e.Name())
		if err != nil {
			return nil, err
		}
		if _, err = part.Write(data); err != nil {
			return nil, err
		}
		fileNames = append(fileNames, e.Name())
	}
	writer.Close()

	if len(fileNames) == 0 {
		return nil, nil
	}

	req, err := http.NewRequest("POST", "https://api.imgbb.com/1/upload?key="+apiKey, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var bulkResp struct {
		Data []struct {
			URL       string `json:"url"`
			DeleteURL string `json:"delete_url"`
			Thumb     struct {
				URL string `json:"url"`
			} `json:"thumb"`
		} `json:"data"`
		Success bool `json:"success"`
		Status  int  `json:"status"`
	}
	if err := json.Unmarshal(respBody, &bulkResp); err != nil {
		return nil, fmt.Errorf("failed to parse bulk response: %w", err)
	}
	if !bulkResp.Success {
		return nil, fmt.Errorf("imgbb bulk upload failed with status %d", bulkResp.Status)
	}

	results := make([]BulkUploadFileResult, len(bulkResp.Data))
	for i, d := range bulkResp.Data {
		name := ""
		if i < len(fileNames) {
			name = fileNames[i]
		}
		results[i] = BulkUploadFileResult{
			FileName: name,
			Result: &ImageUploadResult{
				URL:          d.URL,
				ThumbnailURL: d.Thumb.URL,
				DeleteURL:    d.DeleteURL,
			},
		}
	}
	return results, nil
}

type ImgbbResponse struct {
	Data struct {
		URL       string `json:"url"`
		DeleteURL string `json:"delete_url"`
		Thumb     struct {
			URL string `json:"url"`
		} `json:"thumb"`
	} `json:"data"`
	Success bool `json:"success"`
	Status  int  `json:"status"`
}

func UploadImageToImgbb(imageData []byte, db *sql.DB) (*ImageUploadResult, error) {
	apiKey, err := GetGlobalSetting("imgbb_api_key", db)
	if err != nil || apiKey == "" {
		return nil, fmt.Errorf("imgbb_api_key is not configured")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "upload")
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(imageData); err != nil {
		return nil, err
	}
	writer.Close()

	req, err := http.NewRequest("POST", "https://api.imgbb.com/1/upload?key="+apiKey, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var imgbbResp ImgbbResponse
	if err := json.Unmarshal(respBody, &imgbbResp); err != nil {
		return nil, err
	}

	if !imgbbResp.Success {
		fmt.Printf("imgbb error response: %s\n", string(respBody))
		return nil, fmt.Errorf("imgbb upload failed with status %d", imgbbResp.Status)
	}

	return &ImageUploadResult{
		URL:          imgbbResp.Data.URL,
		ThumbnailURL: imgbbResp.Data.Thumb.URL,
		DeleteURL:    imgbbResp.Data.DeleteURL,
	}, nil
}
