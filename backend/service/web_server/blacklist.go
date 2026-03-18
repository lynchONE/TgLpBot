package web_server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"TgLpBot/service/blacklist"
)

type BlacklistRequest struct {
	InitData    string `json:"initData"`
	PoolAddress string `json:"pool_address"`
	Action      string `json:"action"`
}

type BlacklistResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message,omitempty"`
	Blacklist []string `json:"blacklist,omitempty"`
	Count     int64    `json:"count,omitempty"`
}

func handleBlacklist(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		handleGetBlacklist(w, r)
	case http.MethodPost:
		handleModifyBlacklist(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "unsupported method",
		})
	}
}

func handleGetBlacklist(w http.ResponseWriter, r *http.Request) {
	initData := initDataFromQuery(r)
	if initData == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "missing initData",
		})
		return
	}

	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}

	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil || status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}

	svc := blacklist.NewBlacklistService()
	list, err := svc.GetAll(user.ID)
	if err != nil {
		log.Printf("[Blacklist API] get failed: user_id=%d err=%v", user.ID, err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "failed to get blacklist: " + err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(BlacklistResponse{
		Success:   true,
		Blacklist: list,
		Count:     int64(len(list)),
	})
}

func handleModifyBlacklist(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)

	var req BlacklistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "invalid request body",
		})
		return
	}

	if strings.TrimSpace(req.InitData) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "missing initData",
		})
		return
	}
	if strings.TrimSpace(req.PoolAddress) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "missing pool_address",
		})
		return
	}

	user, status, msg := authenticateTelegramWebAppUser(req.InitData)
	if status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}

	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil || status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}

	svc := blacklist.NewBlacklistService()
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "add":
		if err := svc.Add(user.ID, req.PoolAddress); err != nil {
			log.Printf("[Blacklist API] add failed: user_id=%d pool=%s err=%v", user.ID, req.PoolAddress, err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(BlacklistResponse{
				Success: false,
				Message: "failed to add blacklist item: " + err.Error(),
			})
			return
		}
		_ = json.NewEncoder(w).Encode(BlacklistResponse{
			Success: true,
			Message: "added",
			Count:   svc.Count(user.ID),
		})
	case "remove":
		if err := svc.Remove(user.ID, req.PoolAddress); err != nil {
			log.Printf("[Blacklist API] remove failed: user_id=%d pool=%s err=%v", user.ID, req.PoolAddress, err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(BlacklistResponse{
				Success: false,
				Message: "failed to remove blacklist item: " + err.Error(),
			})
			return
		}
		_ = json.NewEncoder(w).Encode(BlacklistResponse{
			Success: true,
			Message: "removed",
			Count:   svc.Count(user.ID),
		})
	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "invalid action",
		})
	}
}
