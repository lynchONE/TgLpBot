package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	alphaOverviewDataURL      = "https://alpha123.uk/api/data?fresh=1"
	alphaOverviewStabilityURL = "https://alpha123.uk/stability/stability_feed_v3.json"
	alphaOverviewTimeout      = 6 * time.Second
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

func fetchAlphaOverviewSource(ctx context.Context, client *http.Client, source alphaOverviewSource) (json.RawMessage, error) {
	if source.URL == "" {
		return nil, fmt.Errorf("missing alpha source url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
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
