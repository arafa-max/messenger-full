package linkpreview

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
)

type Preview struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`
	SiteName    string `json:"site_name"`
}

type Service struct {
	redis  *redis.Client
	client *http.Client
}

func NewService(rdb *redis.Client) *Service {
	return &Service{
		redis: rdb,
		client: &http.Client{
			Timeout:       5 * time.Second,
			CheckRedirect: safeRedirect,
		},
	}
}

func (s *Service) Fetch(ctx context.Context, rawURL string) (*Preview, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(rawURL)))
	key := "link_preview:" + hash

	cached, err := s.redis.Get(ctx, key).Result()
	if err == nil {
		var p Preview
		json.Unmarshal([]byte(cached), &p)
		return &p, nil
	}

	preview, err := s.parse(rawURL)
	if err != nil {
		return nil, err
	}

	data, _ := json.Marshal(preview)
	s.redis.Set(ctx, key, data, 24*time.Hour)

	return preview, nil
}

func (s *Service) parse(rawURL string) (*Preview, error) {
	if err := validateURL(rawURL); err != nil {
		return nil, err
	}

	resp, err := s.client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	html := string(body)

	return &Preview{
		URL:         rawURL,
		Title:       extractMeta(html, "og:title", "title"),
		Description: extractMeta(html, "og:description", "description"),
		ImageURL:    extractMeta(html, "og:image", ""),
		SiteName:    extractMeta(html, "og:site_name", ""),
	}, nil
}

func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid scheme")
	}
	ips, err := net.LookupHost(u.Hostname())
	if err != nil {
		return err
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("private address blocked")
		}
	}
	return nil
}

func safeRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 {
		return fmt.Errorf("too many redirects")
	}
	return validateURL(req.URL.String())
}

var (
	ogRegex    = regexp.MustCompile(`(?i)<meta[^>]+property="([^"]+)"[^>]+content="([^"]*)"`)
	nameRegex  = regexp.MustCompile(`(?i)<meta[^>]+name="([^"]+)"[^>]+content="([^"]*)"`)
	titleRegex = regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)
)

func extractMeta(html, ogKey, nameKey string) string {
	for _, m := range ogRegex.FindAllStringSubmatch(html, -1) {
		if m[1] == ogKey {
			return m[2]
		}
	}
	if nameKey != "" {
		for _, m := range nameRegex.FindAllStringSubmatch(html, -1) {
			if m[1] == nameKey {
				return m[2]
			}
		}
		if nameKey == "title" {
			if m := titleRegex.FindStringSubmatch(html); len(m) > 1 {
				return m[1]
			}
		}
	}
	return ""
}

var URLRegex = regexp.MustCompile(`https?://[^\s<>"{}|\\^\[\]` + "`" + `]+`)