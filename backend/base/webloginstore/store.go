package webloginstore

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"TgLpBot/base/models"
)

type Entry struct {
	Code      string
	CreatedAt time.Time
	Confirmed bool
	User      *models.User
	PhotoURL  string
}

const (
	CodeTTL    = 3 * time.Minute
	codeLength = 6
)

var (
	codes   = map[string]*Entry{}
	codesMu sync.Mutex
)

func cleanExpired() {
	now := time.Now()
	for code, entry := range codes {
		if now.Sub(entry.CreatedAt) > CodeTTL {
			delete(codes, code)
		}
	}
}

func GenerateCode() (string, error) {
	codesMu.Lock()
	defer codesMu.Unlock()
	cleanExpired()

	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := strings.ToUpper(hex.EncodeToString(b)[:codeLength])
	codes[code] = &Entry{
		Code:      code,
		CreatedAt: time.Now(),
	}
	return code, nil
}

func Confirm(code string, user *models.User, photoURL string) (bool, string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return false, "验证码不能为空"
	}

	codesMu.Lock()
	defer codesMu.Unlock()
	cleanExpired()

	entry, ok := codes[code]
	if !ok {
		return false, "验证码不存在或已过期，请在网页上重新获取"
	}
	if entry.Confirmed {
		return false, "该验证码已被使用"
	}

	entry.Confirmed = true
	entry.User = user
	entry.PhotoURL = photoURL
	return true, ""
}

func Check(code string) (*Entry, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, false
	}

	codesMu.Lock()
	defer codesMu.Unlock()

	entry, ok := codes[code]
	if !ok {
		return nil, false
	}
	if time.Since(entry.CreatedAt) > CodeTTL {
		delete(codes, code)
		return nil, false
	}
	if !entry.Confirmed {
		return entry, false
	}

	// consume
	delete(codes, code)
	return entry, true
}
