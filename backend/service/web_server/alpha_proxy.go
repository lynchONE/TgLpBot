package web_server

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

const (
	alphaOverviewDataURL      = "https://alpha123.uk/api/data?fresh=1"
	alphaOverviewStabilityURL = "https://alpha123.uk/stability/stability_feed_v3.json"
	alphaOverviewTimeout      = 6 * time.Second
	alphaOverviewBrowserUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"
)

var alphaOverviewHTTPClient = &http.Client{Timeout: alphaOverviewTimeout}

type alphaOverviewSource struct {
	Name string
	URL  string
}

type alphaOverviewResponse struct {
	Data      json.RawMessage   `json:"data,omitempty"`
	Stability json.RawMessage   `json:"stability,omitempty"`
	Errors    map[string]string `json:"errors,omitempty"`
}

func alphaPostmanToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func setAlphaOverviewHeaders(req *http.Request) error {
	profile := strings.TrimSpace(os.Getenv("ALPHA_FETCH_PROFILE"))
	if profile == "" || profile == "browser" {
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Pragma", "no-cache")
		req.Header.Set("Referer", "https://alpha123.uk/")
		req.Header.Set("Sec-CH-UA", `"Google Chrome";v="137", "Chromium";v="137", "Not/A)Brand";v="24"`)
		req.Header.Set("Sec-CH-UA-Mobile", "?0")
		req.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("User-Agent", alphaOverviewBrowserUA)
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		req.Header.Set("Connection", "keep-alive")
		return nil
	}

	if profile != "postman" {
		return fmt.Errorf("unsupported ALPHA_FETCH_PROFILE %q", profile)
	}

	token, err := alphaPostmanToken()
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "PostmanRuntime/7.51.1")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Postman-Token", token)
	return nil
}

func alphaOverviewBodyReader(resp *http.Response) (io.Reader, io.Closer, error) {
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "", "identity":
		return resp.Body, nil, nil
	case "gzip":
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, nil, err
		}
		return reader, reader, nil
	case "deflate":
		reader := flate.NewReader(resp.Body)
		return reader, reader, nil
	case "br":
		return brotli.NewReader(resp.Body), nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported content-encoding %q", resp.Header.Get("Content-Encoding"))
	}
}

func fetchAlphaOverviewSource(ctx context.Context, client *http.Client, source alphaOverviewSource) (json.RawMessage, error) {
	if source.URL == "" {
		return nil, fmt.Errorf("missing alpha source url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, err
	}
	if err := setAlphaOverviewHeaders(req); err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyReader, bodyCloser, err := alphaOverviewBodyReader(resp)
	if err != nil {
		return nil, err
	}
	if bodyCloser != nil {
		defer bodyCloser.Close()
	}

	body, err := io.ReadAll(io.LimitReader(bodyReader, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("invalid json")
	}
	return json.RawMessage(body), nil
}

func loadAlphaOverview(ctx context.Context, client *http.Client, sources []alphaOverviewSource) alphaOverviewResponse {
	out := alphaOverviewResponse{}
	errors := map[string]string{}
	for _, source := range sources {
		body, err := fetchAlphaOverviewSource(ctx, client, source)
		if err != nil {
			errors[source.Name] = err.Error()
			continue
		}
		switch source.Name {
		case "data":
			out.Data = body
		case "stability":
			out.Stability = body
		default:
			errors[source.Name] = "unknown alpha source"
		}
	}
	if len(errors) > 0 {
		out.Errors = errors
	}
	return out
}

func (s *Server) handleAlphaOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), alphaOverviewTimeout)
	defer cancel()

	resp := loadAlphaOverview(ctx, alphaOverviewHTTPClient, []alphaOverviewSource{
		{Name: "data", URL: alphaOverviewDataURL},
		{Name: "stability", URL: alphaOverviewStabilityURL},
	})
	if len(resp.Data) == 0 && len(resp.Stability) == 0 {
		writeJSON(w, http.StatusBadGateway, resp)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, resp)
}
