
package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const theresavYtmp4URL = "https://api.theresav.eu/api/download/ytmp4"

const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

func theresavAPIKey() string {
	if k := strings.TrimSpace(os.Getenv("THERESAV_APIKEY")); k != "" {
		return k
	}
	return strings.TrimSpace(TheresavAPIKey)
}

var downloadClient = &http.Client{
	Timeout: 300 * time.Second,
	Transport: &http.Transport{
		ForceAttemptHTTP2: false,
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
	},
}

type videoTrack struct {
	Status      bool   `json:"status"`
	Creator     string `json:"creator"`
	Title       string `json:"title"`
	Channel     string `json:"channel"`
	DurationSec int    `json:"duration_sec"`
	Resolution  string `json:"resolution"`
	Ext         string `json:"ext"`
	Filesize    int64  `json:"filesize"`
	Thumbnail   string `json:"thumbnail"`
	DownloadURL string `json:"download_url"`
}

func fetchVideoPlay(ytURL string) (*videoTrack, error) {
	apiKey := theresavAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("API key Theresav kosong: isi THERESAV_APIKEY atau TheresavAPIKey di config.go")
	}

	ytURL = strings.TrimSpace(ytURL)
	if ytURL == "" {
		return nil, fmt.Errorf("URL video kosong")
	}

	params := url.Values{}
	params.Set("url", ytURL)
	params.Set("resolution", TheresavResolution)

	endpoint := theresavYtmp4URL + "?" + params.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat request: %w", err)
	}

	req.Header.Set("x-apikey", apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", browserUA)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request API gagal: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("gagal membaca respons API: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed videoTrack
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("respons JSON API tidak valid: %w", err)
	}

	if !parsed.Status {
		return nil, fmt.Errorf("API gagal memproses video: %s", strings.TrimSpace(string(body)))
	}

	if strings.TrimSpace(parsed.DownloadURL) == "" {
		return nil, fmt.Errorf("API berhasil merespons tetapi download_url kosong")
	}

	return &parsed, nil
}

func downloadVideo(downloadURL string) (string, error) {
	if strings.TrimSpace(downloadURL) == "" {
		return "", fmt.Errorf("download URL kosong")
	}

	if err := os.MkdirAll("temp", 0755); err != nil {
		return "", err
	}

	path := filepath.Join("temp", uuid.New().String()+".mp4")

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		lastErr = downloadOnce(downloadURL, path)
		if lastErr == nil {
			return path, nil
		}

		fmt.Printf("[downloadVideo] percobaan %d gagal: %v\n", attempt, lastErr)
		_ = os.Remove(path)

		if attempt < 3 {
			time.Sleep(2 * time.Second)
		}
	}

	return "", lastErr
}

func downloadOnce(downloadURL, path string) error {
	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "*/*")

	resp, err := downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("request download gagal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("download status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		_ = os.Remove(path)
		return fmt.Errorf("gagal menyimpan video: %w", err)
	}

	if err := out.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}

	return nil
}
