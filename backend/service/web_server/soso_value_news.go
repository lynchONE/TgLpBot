package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	sosoValueNewsRetention       = 24 * time.Hour
	defaultSosoValueNewsBaseURL  = "https://openapi.sosovalue.com"
	defaultSosoValueFeaturedPath = "/api/v1/news/featured/currency"
	sosoValueFeedFeatured        = "featured"
	sosoValueFeedTicker          = "ticker"
)

type SosoValueNewsService struct {
	client *http.Client

	stopCh   chan struct{}
	stopOnce sync.Once
	ticker   *time.Ticker
}

func NewSosoValueNewsService() *SosoValueNewsService {
	return &SosoValueNewsService{
		client: &http.Client{Timeout: 20 * time.Second},
		stopCh: make(chan struct{}),
	}
}

func (s *SosoValueNewsService) Start() {
	if s == nil {
		return
	}
	if config.AppConfig != nil && !config.AppConfig.SoSoValueNewsSyncEnabled {
		log.Println("[SoSoValueNews] disabled")
		return
	}
	interval := time.Minute
	if config.AppConfig != nil && config.AppConfig.SoSoValueNewsSyncIntervalSeconds > 0 {
		interval = time.Duration(config.AppConfig.SoSoValueNewsSyncIntervalSeconds) * time.Second
	}
	s.ticker = time.NewTicker(interval)
	go func() {
		s.runOnce()
		for {
			select {
			case <-s.stopCh:
				return
			case <-s.ticker.C:
				s.runOnce()
			}
		}
	}()
}

func (s *SosoValueNewsService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		if s.ticker != nil {
			s.ticker.Stop()
		}
	})
}

func (s *SosoValueNewsService) runOnce() {
	if database.DB == nil {
		log.Println("[SoSoValueNews] skipped: mysql not initialized")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := s.cleanupExpired(ctx); err != nil {
		log.Printf("[SoSoValueNews] cleanup failed: %v", err)
	}

	apiKey := s.apiKey()
	if apiKey == "" {
		log.Println("[SoSoValueNews] skipped: SOSO_VALUE_API_KEY is not configured")
		return
	}

	start := time.Now()
	featured, err := s.fetchAndPersist(ctx, sosoValueFeedFeatured, s.featuredEndpoint(), apiKey)
	if err != nil {
		log.Printf("[SoSoValueNews] sync featured failed: %v", err)
	}

	tickerEndpoint := s.tickerEndpoint()
	tickerCount := 0
	if tickerEndpoint != "" && tickerEndpoint != s.featuredEndpoint() {
		tickerCount, err = s.fetchAndPersist(ctx, sosoValueFeedTicker, tickerEndpoint, apiKey)
		if err != nil {
			log.Printf("[SoSoValueNews] sync ticker failed: %v", err)
		}
	} else if err := s.copyFeaturedToTicker(ctx); err != nil {
		log.Printf("[SoSoValueNews] copy featured to ticker failed: %v", err)
	} else {
		tickerCount = featured
	}

	log.Printf("[SoSoValueNews] sync done featured=%d ticker=%d in %s", featured, tickerCount, time.Since(start).String())
}

func (s *SosoValueNewsService) apiKey() string {
	if config.AppConfig == nil {
		return ""
	}
	return strings.TrimSpace(config.AppConfig.SoSoValueAPIKey)
}

func (s *SosoValueNewsService) baseURL() string {
	if config.AppConfig == nil {
		return defaultSosoValueNewsBaseURL
	}
	raw := strings.TrimRight(strings.TrimSpace(config.AppConfig.SoSoValueAPIBaseURL), "/")
	if raw == "" {
		return defaultSosoValueNewsBaseURL
	}
	return raw
}

func (s *SosoValueNewsService) featuredEndpoint() string {
	if config.AppConfig == nil || strings.TrimSpace(config.AppConfig.SoSoValueNewsFeaturedEndpoint) == "" {
		return defaultSosoValueFeaturedPath
	}
	return strings.TrimSpace(config.AppConfig.SoSoValueNewsFeaturedEndpoint)
}

func (s *SosoValueNewsService) tickerEndpoint() string {
	if config.AppConfig == nil {
		return ""
	}
	return strings.TrimSpace(config.AppConfig.SoSoValueNewsTickerEndpoint)
}

func (s *SosoValueNewsService) pageSize() int {
	if config.AppConfig == nil || config.AppConfig.SoSoValueNewsPageSize <= 0 {
		return 30
	}
	if config.AppConfig.SoSoValueNewsPageSize > 100 {
		return 100
	}
	return config.AppConfig.SoSoValueNewsPageSize
}

func (s *SosoValueNewsService) monthlySafetyLimit() int {
	if config.AppConfig == nil || config.AppConfig.SoSoValueNewsMonthlySafetyLimit <= 0 {
		return 95000
	}
	if config.AppConfig.SoSoValueNewsMonthlySafetyLimit > 100000 {
		return 95000
	}
	return config.AppConfig.SoSoValueNewsMonthlySafetyLimit
}

func (s *SosoValueNewsService) categoryList(feed string) []string {
	raw := ""
	if config.AppConfig != nil {
		raw = config.AppConfig.SoSoValueNewsCategoryList
		if feed == sosoValueFeedTicker && strings.TrimSpace(config.AppConfig.SoSoValueNewsTickerCategoryList) != "" {
			raw = config.AppConfig.SoSoValueNewsTickerCategoryList
		}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return []string{"1", "2", "3", "4", "5", "6", "7", "9", "10"}
	}
	return out
}

func (s *SosoValueNewsService) fetchAndPersist(ctx context.Context, feed string, endpoint string, apiKey string) (int, error) {
	if endpoint == "" {
		return 0, fmt.Errorf("empty endpoint for feed=%s", feed)
	}
	if ok, count, limit, err := s.reserveMonthlyRequest(ctx); err != nil {
		return 0, err
	} else if !ok {
		return 0, fmt.Errorf("monthly SoSoValue request safety limit reached: %d/%d", count, limit)
	}

	items, err := s.fetchItems(ctx, feed, endpoint, apiKey)
	if err != nil {
		return 0, err
	}
	rows := make([]models.SosoValueNewsItem, 0, len(items))
	now := time.Now()
	for _, item := range items {
		row, ok := normalizeSosoValueNewsItem(feed, item, now)
		if ok {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if err := database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "feed"}, {Name: "external_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"title",
				"content",
				"language",
				"source_link",
				"author",
				"author_avatar_url",
				"nick_name",
				"category",
				"feature_image",
				"tags_json",
				"raw_json",
				"release_time",
				"fetched_at",
				"updated_at",
			}),
		}).
		CreateInBatches(rows, 100).Error; err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (s *SosoValueNewsService) reserveMonthlyRequest(ctx context.Context) (bool, int, int, error) {
	if database.DB == nil {
		return false, 0, 0, fmt.Errorf("mysql not initialized")
	}
	limit := s.monthlySafetyLimit()
	month := time.Now().Format("2006-01")
	now := time.Now()

	var count int
	err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var usage models.SosoValueAPIUsage
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("month = ?", month).
			First(&usage).Error
		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}
			usage = models.SosoValueAPIUsage{
				Month:         month,
				RequestCount:  0,
				LastRequestAt: nil,
			}
			if err := tx.Create(&usage).Error; err != nil {
				return err
			}
		}
		if usage.RequestCount >= limit {
			count = usage.RequestCount
			return nil
		}
		usage.RequestCount += 1
		usage.LastRequestAt = &now
		if err := tx.Model(&usage).Updates(map[string]any{
			"request_count":   usage.RequestCount,
			"last_request_at": now,
		}).Error; err != nil {
			return err
		}
		count = usage.RequestCount
		return nil
	})
	if err != nil {
		return false, count, limit, err
	}
	return count <= limit, count, limit, nil
}

func (s *SosoValueNewsService) fetchItems(ctx context.Context, feed string, endpoint string, apiKey string) ([]map[string]any, error) {
	u, err := s.buildURL(endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if !queryHasAny(q, "pageNum", "page_num", "page") {
		q.Set("pageNum", "1")
	}
	if !queryHasAny(q, "pageSize", "page_size", "size", "limit") {
		q.Set("pageSize", strconv.Itoa(s.pageSize()))
	}
	if !queryHasAny(q, "categoryList", "categories", "category") {
		categories := s.categoryList(feed)
		if len(categories) > 0 {
			q.Set("categoryList", strings.Join(categories, ","))
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "TgLpBot/1.0")
	req.Header.Set("x-soso-api-key", apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sosovalue http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode sosovalue response: %w", err)
	}
	items := extractSosoValueNewsItems(payload)
	if len(items) == 0 {
		return nil, fmt.Errorf("sosovalue response contains no news items")
	}
	return items, nil
}

func (s *SosoValueNewsService) buildURL(endpoint string) (*url.URL, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("empty endpoint")
	}
	if strings.HasPrefix(strings.ToLower(endpoint), "http://") || strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		return url.Parse(endpoint)
	}
	base := strings.TrimRight(s.baseURL(), "/")
	path := endpoint
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return url.Parse(base + path)
}

func queryHasAny(q url.Values, keys ...string) bool {
	for _, key := range keys {
		if strings.TrimSpace(q.Get(key)) != "" {
			return true
		}
	}
	return false
}

func extractSosoValueNewsItems(payload any) []map[string]any {
	if rows, ok := payload.([]any); ok {
		return mapsFromArray(rows)
	}
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"data", "items", "list", "records", "result"} {
		value, exists := obj[key]
		if !exists {
			continue
		}
		if rows, ok := value.([]any); ok {
			return mapsFromArray(rows)
		}
		if nested, ok := value.(map[string]any); ok {
			if items := extractSosoValueNewsItems(nested); len(items) > 0 {
				return items
			}
		}
	}
	return nil
}

func mapsFromArray(rows []any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if obj, ok := row.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func normalizeSosoValueNewsItem(feed string, raw map[string]any, now time.Time) (models.SosoValueNewsItem, bool) {
	externalID := firstStringField(raw, "id", "newsId", "news_id", "uuid")
	if externalID == "" {
		externalID = firstStringField(raw, "sourceLink", "source_link", "url", "link")
	}
	title, content, language := sosoValueLocalizedContent(raw)
	if title == "" {
		title = firstStringField(raw, "title", "name")
	}
	if content == "" {
		content = firstStringField(raw, "content", "summary", "description")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return models.SosoValueNewsItem{}, false
	}
	if externalID == "" {
		externalID = title
	}

	rawJSON, err := json.Marshal(raw)
	if err != nil {
		rawJSON = []byte("{}")
	}
	tagsJSON := marshalFieldJSON(raw, "tags", "tagList", "tokenList", "symbols")
	releaseTime := parseSosoValueReleaseTime(raw)
	if releaseTime.IsZero() {
		releaseTime = now
	}

	return models.SosoValueNewsItem{
		Feed:            strings.TrimSpace(feed),
		ExternalID:      externalID,
		Title:           title,
		Content:         content,
		Language:        language,
		SourceLink:      resolveSosoValueSourceLink(raw),
		Author:          firstStringField(raw, "author", "source", "sourceName"),
		AuthorAvatarURL: firstStringField(raw, "authorAvatarUrl", "author_avatar_url", "sourceLogo"),
		NickName:        firstStringField(raw, "nickName", "nickname"),
		Category:        firstIntField(raw, "category"),
		FeatureImage:    firstStringField(raw, "featureImage", "feature_image", "cover", "image"),
		TagsJSON:        tagsJSON,
		RawJSON:         string(rawJSON),
		ReleaseTime:     releaseTime,
		FetchedAt:       now,
	}, true
}

func resolveSosoValueSourceLink(raw map[string]any) string {
	for _, value := range []string{
		firstNestedStringField(raw, []string{"quoteInfo"}, "originalUrl", "original_url"),
		firstStringField(raw, "originalUrl", "original_url", "sourceUrl", "source_url", "externalUrl", "external_url", "articleUrl", "article_url"),
		firstExternalContentLink(raw),
		firstStringField(raw, "sourceLink", "source_link", "url", "link"),
	} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveSosoValueSourceLinkFromRaw(current string, rawJSON string) string {
	current = strings.TrimSpace(current)
	var raw map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return current
	}
	resolved := resolveSosoValueSourceLink(raw)
	if resolved == "" {
		return current
	}
	if current != "" && isSosoValueURL(resolved) && !isSosoValueURL(current) {
		return current
	}
	return resolved
}

func firstNestedStringField(raw map[string]any, path []string, keys ...string) string {
	var current any = raw
	for _, part := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = obj[part]
	}
	obj, ok := current.(map[string]any)
	if !ok {
		return ""
	}
	return firstStringField(obj, keys...)
}

func firstExternalContentLink(raw map[string]any) string {
	contentValues := make([]string, 0, 4)
	if title, content, _ := sosoValueLocalizedContent(raw); title != "" && content != "" {
		contentValues = append(contentValues, content)
	}
	for _, key := range []string{"content", "summary", "description"} {
		if value := firstStringField(raw, key); value != "" {
			contentValues = append(contentValues, value)
		}
	}
	for _, content := range contentValues {
		if href := firstExternalHref(content); href != "" {
			return href
		}
	}
	return ""
}

func firstExternalHref(content string) string {
	root, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return ""
	}
	var walk func(*html.Node) string
	walk = func(node *html.Node) string {
		if node.Type == html.ElementNode && node.Data == "a" {
			for _, attr := range node.Attr {
				if strings.EqualFold(attr.Key, "href") {
					href := strings.TrimSpace(attr.Val)
					if href != "" && !isSosoValueURL(href) {
						return href
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if href := walk(child); href != "" {
				return href
			}
		}
		return ""
	}
	return walk(root)
}

func isSosoValueURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "sosovalue.com" || host == "sosovalue.xyz" || strings.HasSuffix(host, ".sosovalue.com") || strings.HasSuffix(host, ".sosovalue.xyz")
}

func sosoValueLocalizedContent(raw map[string]any) (string, string, string) {
	value, ok := raw["multilanguageContent"]
	if !ok {
		value = raw["multiLanguageContent"]
	}
	if !ok && value == nil {
		return "", "", ""
	}
	contentMap, ok := value.(map[string]any)
	if ok {
		preferred := []string{"zh-CN", "zh_CN", "zh", "cn", "en-US", "en_US", "en"}
		for _, key := range preferred {
			if title, content := contentFromLanguageEntry(contentMap[key]); title != "" {
				return title, content, key
			}
		}
		for key, value := range contentMap {
			if title, content := contentFromLanguageEntry(value); title != "" {
				return title, content, key
			}
		}
		return "", "", ""
	}

	contentArray, ok := value.([]any)
	if !ok {
		return "", "", ""
	}
	preferred := []string{"zh-CN", "zh_CN", "zh", "cn", "en-US", "en_US", "en"}
	entries := make([]map[string]any, 0, len(contentArray))
	for _, item := range contentArray {
		if entry, ok := item.(map[string]any); ok {
			entries = append(entries, entry)
		}
	}
	for _, preferredLanguage := range preferred {
		for _, entry := range entries {
			language := firstStringField(entry, "language", "lang", "locale")
			if strings.EqualFold(language, preferredLanguage) {
				if title, content := contentFromLanguageEntry(entry); title != "" {
					return title, content, language
				}
			}
		}
	}
	for _, entry := range entries {
		language := firstStringField(entry, "language", "lang", "locale")
		if title, content := contentFromLanguageEntry(entry); title != "" {
			return title, content, language
		}
	}
	return "", "", ""
}

func contentFromLanguageEntry(value any) (string, string) {
	obj, ok := value.(map[string]any)
	if !ok {
		return "", ""
	}
	title := firstStringField(obj, "title", "name")
	content := firstStringField(obj, "content", "summary", "description")
	return title, content
}

func firstStringField(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			if text := strings.TrimSpace(v); text != "" {
				return text
			}
		case json.Number:
			if text := strings.TrimSpace(v.String()); text != "" {
				return text
			}
		case float64:
			if v != 0 {
				return strconv.FormatInt(int64(v), 10)
			}
		}
	}
	return ""
}

func firstIntField(raw map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case float64:
			return int(v)
		case json.Number:
			n, _ := strconv.Atoi(v.String())
			return n
		case string:
			n, _ := strconv.Atoi(strings.TrimSpace(v))
			return n
		}
	}
	return 0
}

func marshalFieldJSON(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		b, err := json.Marshal(value)
		if err == nil && strings.TrimSpace(string(b)) != "" {
			return string(b)
		}
	}
	return "[]"
}

func parseSosoValueReleaseTime(raw map[string]any) time.Time {
	for _, key := range []string{"releaseTime", "release_time", "publishedAt", "publishTime", "createdAt"} {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		if ts := parseLooseTime(value); !ts.IsZero() {
			return ts
		}
	}
	return time.Time{}
}

func parseLooseTime(value any) time.Time {
	switch v := value.(type) {
	case float64:
		return unixTimeFromNumber(v)
	case json.Number:
		f, _ := strconv.ParseFloat(v.String(), 64)
		return unixTimeFromNumber(f)
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return time.Time{}
		}
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			return unixTimeFromNumber(f)
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
			if ts, err := time.Parse(layout, raw); err == nil {
				return ts
			}
		}
	}
	return time.Time{}
}

func unixTimeFromNumber(value float64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value > 1_000_000_000_000 {
		return time.UnixMilli(int64(value)).UTC()
	}
	return time.Unix(int64(value), 0).UTC()
}

func (s *SosoValueNewsService) copyFeaturedToTicker(ctx context.Context) error {
	var rows []models.SosoValueNewsItem
	cutoff := time.Now().Add(-sosoValueNewsRetention)
	if err := database.DB.WithContext(ctx).
		Where("feed = ? AND release_time >= ?", sosoValueFeedFeatured, cutoff).
		Order("release_time DESC").
		Limit(s.pageSize()).
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	now := time.Now()
	copies := make([]models.SosoValueNewsItem, 0, len(rows))
	for _, row := range rows {
		row.ID = 0
		row.Feed = sosoValueFeedTicker
		row.FetchedAt = now
		copies = append(copies, row)
	}
	return database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "feed"}, {Name: "external_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"title",
				"content",
				"language",
				"source_link",
				"author",
				"author_avatar_url",
				"nick_name",
				"category",
				"feature_image",
				"tags_json",
				"raw_json",
				"release_time",
				"fetched_at",
				"updated_at",
			}),
		}).
		CreateInBatches(copies, 100).Error
}

func (s *SosoValueNewsService) cleanupExpired(ctx context.Context) error {
	if database.DB == nil {
		return nil
	}
	cutoff := time.Now().Add(-sosoValueNewsRetention)
	return database.DB.WithContext(ctx).
		Where("(release_time < ? OR fetched_at < ?)", cutoff, cutoff).
		Delete(&models.SosoValueNewsItem{}).Error
}

type sosoValueNewsResponse struct {
	Items       []sosoValueNewsDTO `json:"items"`
	UpdatedAt   string             `json:"updated_at"`
	Status      string             `json:"status"`
	Message     string             `json:"message,omitempty"`
	Usage       sosoValueUsageDTO  `json:"usage"`
	GeneratedAt string             `json:"generated_at"`
}

type sosoValueUsageDTO struct {
	Month        string `json:"month"`
	RequestCount int    `json:"request_count"`
	SafetyLimit  int    `json:"safety_limit"`
}

type sosoValueNewsDTO struct {
	ID              uint   `json:"id"`
	Feed            string `json:"feed"`
	ExternalID      string `json:"external_id"`
	Title           string `json:"title"`
	Content         string `json:"content"`
	Language        string `json:"language"`
	SourceLink      string `json:"source_link"`
	Author          string `json:"author"`
	AuthorAvatarURL string `json:"author_avatar_url"`
	NickName        string `json:"nick_name"`
	Category        int    `json:"category"`
	FeatureImage    string `json:"feature_image"`
	ReleaseTime     string `json:"release_time"`
	FetchedAt       string `json:"fetched_at"`
}

func (s *Server) handleSosoValueNews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if database.DB == nil {
		http.Error(w, "mysql not initialized", http.StatusInternalServerError)
		return
	}

	feed := normalizeSosoValueFeed(r.URL.Query().Get("feed"))
	limit := parseNewsLimit(r.URL.Query().Get("limit"), feed)
	cutoff := time.Now().Add(-sosoValueNewsRetention)

	var rows []models.SosoValueNewsItem
	if err := database.DB.WithContext(r.Context()).
		Where("feed = ? AND release_time >= ? AND fetched_at >= ?", feed, cutoff, cutoff).
		Order("release_time DESC").
		Limit(newsQueryLimit(limit)).
		Find(&rows).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]sosoValueNewsDTO, 0, limit)
	seenTitles := make(map[string]struct{}, len(rows))
	updatedAt := time.Time{}
	for _, row := range rows {
		titleKey := normalizeNewsTitleKey(row.Title)
		if titleKey != "" {
			if _, exists := seenTitles[titleKey]; exists {
				continue
			}
			seenTitles[titleKey] = struct{}{}
		}
		sourceLink := resolveSosoValueSourceLinkFromRaw(row.SourceLink, row.RawJSON)
		items = append(items, sosoValueNewsDTO{
			ID:              row.ID,
			Feed:            row.Feed,
			ExternalID:      row.ExternalID,
			Title:           row.Title,
			Content:         row.Content,
			Language:        row.Language,
			SourceLink:      sourceLink,
			Author:          row.Author,
			AuthorAvatarURL: row.AuthorAvatarURL,
			NickName:        row.NickName,
			Category:        row.Category,
			FeatureImage:    row.FeatureImage,
			ReleaseTime:     row.ReleaseTime.Format(time.RFC3339),
			FetchedAt:       row.FetchedAt.Format(time.RFC3339),
		})
		if row.FetchedAt.After(updatedAt) {
			updatedAt = row.FetchedAt
		}
		if len(items) >= limit {
			break
		}
	}

	status := "ok"
	message := ""
	if len(items) == 0 {
		status = "empty"
		message = "no cached SoSoValue news in the last 24 hours"
	}
	if config.AppConfig == nil || strings.TrimSpace(config.AppConfig.SoSoValueAPIKey) == "" {
		if len(items) == 0 {
			status = "unconfigured"
		}
		message = "SOSO_VALUE_API_KEY is not configured"
	}

	writeJSON(w, http.StatusOK, sosoValueNewsResponse{
		Items:       items,
		UpdatedAt:   formatOptionalRFC3339(updatedAt),
		Status:      status,
		Message:     message,
		Usage:       sosoValueUsage(r.Context()),
		GeneratedAt: time.Now().Format(time.RFC3339),
	})
}

func normalizeSosoValueFeed(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case sosoValueFeedTicker:
		return sosoValueFeedTicker
	default:
		return sosoValueFeedFeatured
	}
}

func parseNewsLimit(raw string, feed string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		if feed == sosoValueFeedTicker {
			return 24
		}
		return 6
	}
	if n > 50 {
		return 50
	}
	return n
}

func newsQueryLimit(limit int) int {
	if limit <= 0 {
		return 24
	}
	queryLimit := limit * 4
	if queryLimit < 20 {
		queryLimit = 20
	}
	if queryLimit > 100 {
		queryLimit = 100
	}
	return queryLimit
}

var newsTitleKeyReplacer = regexp.MustCompile(`[\s[:punct:]\p{P}\p{S}]+`)

func normalizeNewsTitleKey(title string) string {
	key := strings.ToLower(strings.TrimSpace(title))
	if key == "" {
		return ""
	}
	key = newsTitleKeyReplacer.ReplaceAllString(key, "")
	return key
}

func formatOptionalRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func sosoValueUsage(ctx context.Context) sosoValueUsageDTO {
	limit := 95000
	if config.AppConfig != nil && config.AppConfig.SoSoValueNewsMonthlySafetyLimit > 0 {
		limit = config.AppConfig.SoSoValueNewsMonthlySafetyLimit
		if limit > 100000 {
			limit = 95000
		}
	}
	month := time.Now().Format("2006-01")
	usage := sosoValueUsageDTO{Month: month, SafetyLimit: limit}
	if database.DB == nil {
		return usage
	}
	var row models.SosoValueAPIUsage
	if err := database.DB.WithContext(ctx).Where("month = ?", month).First(&row).Error; err == nil {
		usage.RequestCount = row.RequestCount
	}
	return usage
}
