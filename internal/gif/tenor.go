package gif

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Service struct {
	apiKey string
	client *http.Client
}

func NewService(apiKey string) *Service {
	return &Service{
		apiKey: apiKey,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

type GIF struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	ThumbURL string `json:"thumb_url"`
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]GIF, error) {
	u := fmt.Sprintf(
		"https://tenor.googleapis.com/v2/search?q=%s&key=%s&limit=%d&media_filter=mp4,tinygif",
		url.QueryEscape(query), s.apiKey, limit,
	)
	return s.fetch(ctx, u)
}

func (s *Service) Trending(ctx context.Context, limit int) ([]GIF, error) {
	u := fmt.Sprintf(
		"https://tenor.googleapis.com/v2/featured?key=%s&limit=%d&media_filter=mp4,tinygif",
		s.apiKey, limit,
	)
	return s.fetch(ctx, u)
}

func (s *Service) fetch(ctx context.Context, u string) ([]GIF, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			ID           string                       `json:"id"`
			Title        string                       `json:"title"`
			MediaFormats map[string]struct {
				URL string `json:"url"`
			} `json:"media_formats"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	gifs := make([]GIF, 0, len(result.Results))
	for _, r := range result.Results {
		g := GIF{ID: r.ID, Title: r.Title}
		if mp4, ok := r.MediaFormats["mp4"]; ok {
			g.URL = mp4.URL
		}
		if thumb, ok := r.MediaFormats["tinygif"]; ok {
			g.ThumbURL = thumb.URL
		}
		gifs = append(gifs, g)
	}
	return gifs, nil
}