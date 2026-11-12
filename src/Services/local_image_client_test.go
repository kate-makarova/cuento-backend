package Services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectImageExt(t *testing.T) {
	cases := []struct {
		name    string
		header  []byte
		wantExt string
	}{
		{"PNG", []byte{0x89, 0x50, 0x4E, 0x47}, ".png"},
		{"JPEG", []byte{0xFF, 0xD8, 0xFF, 0xE0}, ".jpg"},
		{"GIF", []byte{0x47, 0x49, 0x46, 0x38}, ".gif"},
		{"WebP", []byte{0x52, 0x49, 0x46, 0x46}, ".webp"},
		{"BMP", []byte{0x42, 0x4D, 0x00, 0x00}, ".bmp"},
		{"unknown", []byte{0x00, 0x01, 0x02, 0x03}, ".jpg"},
		{"too short", []byte{0xFF}, ".jpg"},
	}
	for _, tc := range cases {
		got := detectImageExt(tc.header)
		if got != tc.wantExt {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.wantExt)
		}
	}
}

func TestGetImageClientFactory(t *testing.T) {
	cases := []struct {
		setting string
		wantLocal bool
	}{
		{"local", true},
		{"imgbb", false},
		{"", false},
		{"unknown", false},
	}
	for _, tc := range cases {
		client := getImageClientFromValue(tc.setting)
		_, isLocal := client.(LocalImageClient)
		if isLocal != tc.wantLocal {
			t.Errorf("setting %q: isLocal=%v, want %v (got %T)", tc.setting, isLocal, tc.wantLocal, client)
		}
	}
}

func TestLocalClientUploadToDir(t *testing.T) {
	dir := t.TempDir()
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	result, err := localClientUploadToDir(pngData, "http://example.com:8888", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result.URL, "http://example.com:8888/") {
		t.Errorf("URL %q does not start with base URL", result.URL)
	}
	if !strings.HasSuffix(result.URL, ".png") {
		t.Errorf("URL %q does not end with .png", result.URL)
	}
	if result.URL != result.ThumbnailURL {
		t.Errorf("URL and ThumbnailURL should match for local storage")
	}
	filename := strings.TrimPrefix(result.URL, "http://example.com:8888/")
	if _, err := os.Stat(filepath.Join(dir, filename)); err != nil {
		t.Errorf("file not found on disk: %v", err)
	}
}

func TestLocalClientUploadTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	result, err := localClientUploadToDir([]byte{0xFF, 0xD8}, "http://example.com:8888/", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// strip the scheme prefix before checking for double slashes
	withoutScheme := strings.SplitN(result.URL, "://", 2)
	if len(withoutScheme) == 2 && strings.Contains(withoutScheme[1], "//") {
		t.Errorf("URL %q contains double slash after scheme", result.URL)
	}
}

func TestLocalClientUploadEmptyBaseURL(t *testing.T) {
	_, err := localClientUploadToDir([]byte{0xFF, 0xD8}, "", t.TempDir())
	if err == nil {
		t.Error("expected error for empty base URL, got nil")
	}
}
