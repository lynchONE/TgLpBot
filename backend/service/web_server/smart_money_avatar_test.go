package web_server

import "testing"

func TestDetectSmartMoneyAvatarContentType(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00}
	if got := detectSmartMoneyAvatarContentType(png, ""); got != "image/png" {
		t.Fatalf("unexpected content type: %s", got)
	}
}

func TestIsAllowedSmartMoneyAvatarContentType(t *testing.T) {
	if !isAllowedSmartMoneyAvatarContentType("image/webp") {
		t.Fatalf("expected webp to be allowed")
	}
	if isAllowedSmartMoneyAvatarContentType("text/plain") {
		t.Fatalf("expected text/plain to be rejected")
	}
}
