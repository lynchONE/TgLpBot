package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"strings"
	"testing"
	"time"
)

func TestExtractSosoValueNewsItemsNestedData(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"list": []any{
				map[string]any{"id": "1", "title": "first"},
				map[string]any{"id": "2", "title": "second"},
			},
		},
	}

	items := extractSosoValueNewsItems(payload)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0]["id"] != "1" || items[1]["id"] != "2" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestNormalizeSosoValueNewsItemUsesChineseContentArray(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	raw := map[string]any{
		"id":          "news-1",
		"sourceLink":  "https://example.com/news-1",
		"releaseTime": float64(now.UnixMilli()),
		"multilanguageContent": []any{
			map[string]any{
				"language": "en",
				"title":    "English title",
				"content":  "English body",
			},
			map[string]any{
				"language": "zh",
				"title":    "中文标题",
				"content":  "中文正文",
			},
		},
	}

	row, ok := normalizeSosoValueNewsItem(sosoValueFeedFeatured, raw, now)
	if !ok {
		t.Fatal("normalize returned false")
	}
	if row.Title != "中文标题" {
		t.Fatalf("Title = %q, want 中文标题", row.Title)
	}
	if row.Content != "中文正文" {
		t.Fatalf("Content = %q, want 中文正文", row.Content)
	}
	if row.Language != "zh" {
		t.Fatalf("Language = %q, want zh", row.Language)
	}
	if !row.ReleaseTime.Equal(now) {
		t.Fatalf("ReleaseTime = %s, want %s", row.ReleaseTime, now)
	}
}

func TestNormalizeSosoValueNewsItemPrefersOriginalSourceLink(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	raw := map[string]any{
		"id":         "news-1",
		"title":      "news title",
		"sourceLink": "https://sosovalue.com/zh/research/1",
		"quoteInfo": map[string]any{
			"originalUrl": "https://example.com/original-news",
		},
	}

	row, ok := normalizeSosoValueNewsItem(sosoValueFeedFeatured, raw, now)
	if !ok {
		t.Fatal("normalize returned false")
	}
	if row.SourceLink != "https://example.com/original-news" {
		t.Fatalf("SourceLink = %q, want original source", row.SourceLink)
	}
}

func TestResolveSosoValueSourceLinkFromRawKeepsExternalCurrentOverSosoValue(t *testing.T) {
	rawJSON := `{"sourceLink":"https://sosovalue.com/zh/research/1"}`
	got := resolveSosoValueSourceLinkFromRaw("https://example.com/current", rawJSON)
	if got != "https://example.com/current" {
		t.Fatalf("got %q, want current external link", got)
	}
}

func TestResolveSosoValueSourceLinkFromRawUsesSosoValueWhenCurrentEmpty(t *testing.T) {
	rawJSON := `{"sourceLink":"https://sosovalue.com/zh/research/1"}`
	got := resolveSosoValueSourceLinkFromRaw("", rawJSON)
	if got != "https://sosovalue.com/zh/research/1" {
		t.Fatalf("got %q, want raw source link", got)
	}
}

func TestNormalizeNewsTitleKeyDedupesPunctuationAndSpaces(t *testing.T) {
	left := normalizeNewsTitleKey("BTC 现货 ETF：资金流入扩大")
	right := normalizeNewsTitleKey("btc现货ETF资金流入扩大")
	if left == "" {
		t.Fatal("left key is empty")
	}
	if left != right {
		t.Fatalf("left = %q, right = %q", left, right)
	}
}

func TestNormalizeNewsContentKeyDedupesSameHTMLContent(t *testing.T) {
	left := normalizeNewsContentKey(`<p>BTC 现货 ETF 资金流入扩大，市场风险偏好回升。</p>`)
	right := normalizeNewsContentKey(`BTC现货ETF资金流入扩大市场风险偏好回升`)
	if left == "" {
		t.Fatal("left key is empty")
	}
	if left != right {
		t.Fatalf("left = %q, right = %q", left, right)
	}
}

func TestIsDuplicateNewsRowDedupesByContent(t *testing.T) {
	seen := make(map[string]struct{})
	first := models.SosoValueNewsItem{
		Title:   "source A title",
		Content: "BTC 现货 ETF 资金流入扩大，市场风险偏好回升。",
	}
	second := models.SosoValueNewsItem{
		Title:   "source B different title",
		Content: "BTC现货ETF资金流入扩大市场风险偏好回升",
	}
	if isDuplicateNewsRow(first, "https://example.com/a", seen) {
		t.Fatal("first row detected as duplicate")
	}
	if !isDuplicateNewsRow(second, "https://example.com/b", seen) {
		t.Fatal("second row was not detected as duplicate")
	}
}

func TestTickerEndpointDefaultsToFeaturedNewsEndpoint(t *testing.T) {
	service := NewSosoValueNewsService()
	oldConfig := config.AppConfig
	defer func() { config.AppConfig = oldConfig }()

	config.AppConfig = nil
	if got := service.tickerEndpoint(); got != defaultSosoValueFeaturedPath {
		t.Fatalf("tickerEndpoint = %q, want default endpoint", got)
	}

	config.AppConfig = &config.Config{}
	if got := service.tickerEndpoint(); got != "" {
		t.Fatalf("tickerEndpoint = %q, want explicit empty endpoint", got)
	}
	config.AppConfig.SoSoValueNewsTickerEndpoint = defaultSosoValueFeaturedPath
	if got := service.tickerEndpoint(); got != defaultSosoValueFeaturedPath {
		t.Fatalf("tickerEndpoint = %q, want featured endpoint", got)
	}
}

func TestCategoryListUsesTickerCategory(t *testing.T) {
	service := NewSosoValueNewsService()
	oldConfig := config.AppConfig
	defer func() { config.AppConfig = oldConfig }()
	config.AppConfig = &config.Config{
		SoSoValueNewsCategoryList:       "1,2,3",
		SoSoValueNewsTickerCategoryList: "13",
	}

	featured := service.categoryList(sosoValueFeedFeatured)
	ticker := service.categoryList(sosoValueFeedTicker)
	if strings.Join(featured, ",") != "1,2,3" {
		t.Fatalf("featured categories = %v, want 1,2,3", featured)
	}
	if strings.Join(ticker, ",") != "13" {
		t.Fatalf("ticker categories = %v, want 13", ticker)
	}
}

func TestNormalizeSosoValueNewsItemRejectsMissingTitle(t *testing.T) {
	_, ok := normalizeSosoValueNewsItem(sosoValueFeedFeatured, map[string]any{
		"id": "news-2",
	}, time.Now())
	if ok {
		t.Fatal("normalize returned true for missing title")
	}
}
