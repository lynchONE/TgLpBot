package web_server

import (
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

func TestNormalizeSosoValueNewsItemRejectsMissingTitle(t *testing.T) {
	_, ok := normalizeSosoValueNewsItem(sosoValueFeedFeatured, map[string]any{
		"id": "news-2",
	}, time.Now())
	if ok {
		t.Fatal("normalize returned true for missing title")
	}
}
