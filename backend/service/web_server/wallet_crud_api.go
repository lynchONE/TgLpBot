package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/service/wallet"
)

// --- Wallet CRUD API (import / create / rename / set-default / delete) ---

type walletCRUDRequest struct {
	InitData   string `json:"initData"`
	Action     string `json:"action"`                // import, create, rename, set_default, delete
	PrivateKey string `json:"private_key,omitempty"` // for import
	Name       string `json:"name,omitempty"`        // for import / create / rename
	WalletID   uint   `json:"wallet_id,omitempty"`   // for rename / set_default / delete
}

type walletCRUDResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	// optional fields returned on success
	WalletID uint   `json:"wallet_id,omitempty"`
	Address  string `json:"address,omitempty"`
}

func (s *Server) handleWalletCRUD(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req walletCRUDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "无效的 JSON 请求体", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	walletService := wallet.NewWalletService()

	action := strings.TrimSpace(strings.ToLower(req.Action))
	switch action {
	case "import":
		pk := strings.TrimSpace(req.PrivateKey)
		if pk == "" {
			http.Error(w, "缺少私钥参数", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = "导入钱包"
		}
		wlt, err := walletService.ImportWallet(user.ID, pk, name)
		if err != nil {
			http.Error(w, "导入钱包失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(walletCRUDResponse{
			OK:       true,
			Message:  "钱包导入成功",
			WalletID: wlt.ID,
			Address:  wlt.Address,
		})

	case "create":
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = "新钱包"
		}
		wlt, err := walletService.CreateWallet(user.ID, name)
		if err != nil {
			http.Error(w, "创建钱包失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(walletCRUDResponse{
			OK:       true,
			Message:  "钱包创建成功",
			WalletID: wlt.ID,
			Address:  wlt.Address,
		})

	case "rename":
		if req.WalletID == 0 {
			http.Error(w, "缺少 wallet_id 参数", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			http.Error(w, "缺少 name 参数", http.StatusBadRequest)
			return
		}
		if err := walletService.RenameWallet(user.ID, req.WalletID, name); err != nil {
			http.Error(w, "重命名钱包失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(walletCRUDResponse{OK: true, Message: "钱包重命名成功"})

	case "set_default":
		if req.WalletID == 0 {
			http.Error(w, "缺少 wallet_id 参数", http.StatusBadRequest)
			return
		}
		if err := walletService.SetDefaultWallet(user.ID, req.WalletID); err != nil {
			http.Error(w, "设置默认钱包失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(walletCRUDResponse{OK: true, Message: "默认钱包设置成功"})

	case "delete":
		if req.WalletID == 0 {
			http.Error(w, "缺少 wallet_id 参数", http.StatusBadRequest)
			return
		}
		if err := walletService.DeleteWallet(user.ID, req.WalletID); err != nil {
			http.Error(w, "删除钱包失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(walletCRUDResponse{OK: true, Message: "钱包删除成功"})

	default:
		http.Error(w, "无效的操作: "+req.Action, http.StatusBadRequest)
	}
}
