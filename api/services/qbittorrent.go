package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type QBittorrentClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
	cookie     string
	cookieName string // QBT_SID_<port> in qBittorrent ≥5, "SID" in older versions
	mu         sync.Mutex
}

type QBTorrent struct {
	Hash             string  `json:"hash"`
	Name             string  `json:"name"`
	Size             int64   `json:"size"`
	Downloaded       int64   `json:"downloaded"`
	Progress         float64 `json:"progress"`
	State            string  `json:"state"`
	SavePath         string  `json:"save_path"`
	AddedOn          int64   `json:"added_on"`
	NumSeeds         int     `json:"num_seeds"`
	NumLeechs        int     `json:"num_leechs"`
	DownloadSpeed    int64   `json:"dlspeed"`
	UploadSpeed      int64   `json:"upspeed"`
	Eta              int64   `json:"eta"`
}

type QBTorrentFile struct {
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
}

var QBClient *QBittorrentClient

func NewQBittorrentClient() *QBittorrentClient {
	return &QBittorrentClient{
		baseURL:  os.Getenv("QBITTORRENT_URL"),
		username: os.Getenv("QBITTORRENT_USER"),
		password: os.Getenv("QBITTORRENT_PASS"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *QBittorrentClient) login() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data := url.Values{}
	data.Set("username", c.username)
	data.Set("password", c.password)

	resp, err := c.httpClient.PostForm(c.baseURL+"/api/v2/auth/login", data)
	if err != nil {
		return fmt.Errorf("qBittorrent login failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := strings.TrimSpace(string(body))

	// qBittorrent ≥5 returns 204 No Content on success.
	// Older versions return HTTP 200 with body "Ok.".
	if resp.StatusCode != http.StatusNoContent && bodyStr != "Ok." {
		return fmt.Errorf("qBittorrent login rejected (HTTP %d): %s", resp.StatusCode, bodyStr)
	}

	// qBittorrent ≥5 uses QBT_SID_<port> as the session cookie name.
	// Older versions used "SID". Accept either.
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "SID" || strings.HasPrefix(cookie.Name, "QBT_SID_") {
			c.cookie = cookie.Value
			c.cookieName = cookie.Name
			return nil
		}
	}
	return fmt.Errorf("no session cookie in qBittorrent response")
}

func (c *QBittorrentClient) doRequest(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	if c.cookie == "" {
		if err := c.login(); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}

	// G124: session cookie is for internal server-to-server qBittorrent API calls;
	// browser-only cookie attributes (Secure/HttpOnly/SameSite) are not applicable.
	req.AddCookie(&http.Cookie{Name: c.cookieName, Value: c.cookie}) // #nosec G124 -- internal server-to-server cookie, browser cookie attributes not applicable
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Re-authenticate if session expired
	if resp.StatusCode == http.StatusForbidden {
		// G104: best-effort close; a close error here doesn't affect correctness
		_ = resp.Body.Close()
		if err := c.login(); err != nil {
			return nil, err
		}
		req.AddCookie(&http.Cookie{Name: c.cookieName, Value: c.cookie}) // #nosec G124 -- internal server-to-server cookie, browser cookie attributes not applicable
		return c.httpClient.Do(req)
	}

	return resp, nil
}

func (c *QBittorrentClient) AddMagnet(magnetURL, savePath string) error {
	data := url.Values{}
	data.Set("urls", magnetURL)
	data.Set("savepath", savePath)
	data.Set("autoTMM", "false")

	resp, err := c.doRequest("POST", "/api/v2/torrents/add",
		strings.NewReader(data.Encode()),
		"application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *QBittorrentClient) AddTorrentFile(fileBytes []byte, filename, savePath string) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	_ = writer.WriteField("savepath", savePath)
	_ = writer.WriteField("autoTMM", "false")

	part, err := writer.CreateFormFile("torrents", filename)
	if err != nil {
		return err
	}
	_, _ = part.Write(fileBytes)
	// G104: multipart writer Close flushes boundary — error is non-fatal here
	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing multipart writer: %w", err)
	}

	resp, err := c.doRequest("POST", "/api/v2/torrents/add", &buf, writer.FormDataContentType())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *QBittorrentClient) GetTorrents(hashes []string) ([]QBTorrent, error) {
	path := "/api/v2/torrents/info"
	if len(hashes) > 0 {
		path += "?hashes=" + strings.Join(hashes, "|")
	}

	resp, err := c.doRequest("GET", path, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var torrents []QBTorrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, err
	}
	return torrents, nil
}

func (c *QBittorrentClient) GetTorrent(hash string) (*QBTorrent, error) {
	torrents, err := c.GetTorrents([]string{hash})
	if err != nil {
		return nil, err
	}
	if len(torrents) == 0 {
		return nil, fmt.Errorf("torrent not found: %s", hash)
	}
	return &torrents[0], nil
}

func (c *QBittorrentClient) GetTorrentFiles(hash string) ([]QBTorrentFile, error) {
	resp, err := c.doRequest("GET", "/api/v2/torrents/files?hash="+hash, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var files []QBTorrentFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}
	return files, nil
}

func (c *QBittorrentClient) DeleteTorrent(hash string, deleteFiles bool) error {
	data := url.Values{}
	data.Set("hashes", hash)
	if deleteFiles {
		data.Set("deleteFiles", "true")
	} else {
		data.Set("deleteFiles", "false")
	}

	resp, err := c.doRequest("POST", "/api/v2/torrents/delete",
		strings.NewReader(data.Encode()),
		"application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *QBittorrentClient) PauseTorrent(hash string) error {
	data := url.Values{}
	data.Set("hashes", hash)
	// qBittorrent v5 renamed pause→stop. Use the new endpoint.
	resp, err := c.doRequest("POST", "/api/v2/torrents/stop",
		strings.NewReader(data.Encode()),
		"application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qBittorrent stop failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *QBittorrentClient) ResumeTorrent(hash string) error {
	data := url.Values{}
	data.Set("hashes", hash)
	// qBittorrent v5 renamed resume→start. Use the new endpoint.
	resp, err := c.doRequest("POST", "/api/v2/torrents/start",
		strings.NewReader(data.Encode()),
		"application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qBittorrent start failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
