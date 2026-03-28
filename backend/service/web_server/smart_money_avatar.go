package web_server

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"TgLpBot/base/storage"
)

const smartMoneyAvatarMaxUploadSize = 5 << 20

var smartMoneyAvatarAllowedContentTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
}

var uploadSmartMoneyAvatar = storage.UploadSmartMoneyAvatar

func (s *Server) handleSMWalletAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if smService == nil {
		jsonError(w, "smart money service unavailable", http.StatusInternalServerError)
		return
	}

	repo := smService.Repo()
	if repo == nil {
		jsonError(w, "smart money repository unavailable", http.StatusInternalServerError)
		return
	}

	chainID := resolveSmartMoneyRequestChainID(r)
	address := strings.TrimSpace(r.URL.Query().Get("address"))
	if !isValidAddress(address) {
		jsonError(w, "invalid address", http.StatusBadRequest)
		return
	}

	existing, err := repo.GetMonitoredWalletByAddress(r.Context(), address, chainID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		jsonError(w, "wallet not found", http.StatusNotFound)
		return
	}

	data, contentType, err := readSmartMoneyAvatarUpload(w, r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	avatarURL, err := uploadSmartMoneyAvatar(r.Context(), address, contentType, data)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := repo.UpdateMonitoredWallet(r.Context(), address, chainID, map[string]interface{}{"avatar_url": avatarURL}); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]interface{}{
		"ok":         true,
		"avatar_url": avatarURL,
	})
}

func readSmartMoneyAvatarUpload(w http.ResponseWriter, r *http.Request) ([]byte, string, error) {
	if w != nil {
		r.Body = http.MaxBytesReader(w, r.Body, smartMoneyAvatarMaxUploadSize+(1<<20))
	}
	if err := r.ParseMultipartForm(smartMoneyAvatarMaxUploadSize); err != nil {
		return nil, "", fmt.Errorf("avatar upload exceeds %dMB or is invalid", smartMoneyAvatarMaxUploadSize>>20)
	}

	file, header, err := r.FormFile("avatar")
	if err != nil {
		file, header, err = r.FormFile("file")
	}
	if err != nil {
		return nil, "", fmt.Errorf("avatar file is required")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, smartMoneyAvatarMaxUploadSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("read avatar upload: %w", err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("avatar file is empty")
	}
	if len(data) > smartMoneyAvatarMaxUploadSize {
		return nil, "", fmt.Errorf("avatar file exceeds %dMB", smartMoneyAvatarMaxUploadSize>>20)
	}

	contentType := detectSmartMoneyAvatarContentType(data, "")
	if header != nil {
		contentType = detectSmartMoneyAvatarContentType(data, header.Header.Get("Content-Type"))
	}
	if !isAllowedSmartMoneyAvatarContentType(contentType) {
		return nil, "", fmt.Errorf("unsupported avatar format, only PNG/JPG/WEBP are allowed")
	}

	return data, contentType, nil
}

func detectSmartMoneyAvatarContentType(data []byte, headerValue string) string {
	contentType := http.DetectContentType(data)
	if contentType != "" {
		if idx := strings.Index(contentType, ";"); idx >= 0 {
			contentType = strings.TrimSpace(contentType[:idx])
		}
	}
	if contentType != "" && contentType != "application/octet-stream" {
		return strings.ToLower(contentType)
	}

	contentType = strings.TrimSpace(headerValue)
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	return strings.ToLower(contentType)
}

func isAllowedSmartMoneyAvatarContentType(contentType string) bool {
	_, ok := smartMoneyAvatarAllowedContentTypes[strings.ToLower(strings.TrimSpace(contentType))]
	return ok
}
