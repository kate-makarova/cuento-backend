package Services

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

type ImgbbClient struct{}

func (ImgbbClient) BulkUploadSupport() bool { return false }

func (c ImgbbClient) UploadImage(imageData []byte, db *sql.DB) (*ImageUploadResult, error) {
	return UploadImageToImgbb(imageData, db)
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
