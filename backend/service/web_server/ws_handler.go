package web_server

import (
	"net/http"
	"strings"
)

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil {
		http.Error(w, "websocket not available", http.StatusServiceUnavailable)
		return
	}

	initData := strings.TrimSpace(r.URL.Query().Get("initData"))
	if initData == "" {
		initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
	}

	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	s.Hub.ServeWS(w, r, user.ID)
}
