package user

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AccessService struct{}

func NewAccessService() *AccessService {
	return &AccessService{}
}

type AccessCheck struct {
	Allowed bool
	IsAdmin bool
	Access  *models.UserAccess
	Reason  string
}

func normalizeHexAddress(addr string) string {
	return strings.ToLower(strings.TrimSpace(addr))
}

func (s *AccessService) IsAdminWalletAddress(addr string) bool {
	admin := normalizeHexAddress(config.AppConfig.AdminWalletAddress)
	if admin == "" {
		return false
	}
	return normalizeHexAddress(addr) == admin
}

func (s *AccessService) IsAdminUser(userID uint) bool {
	admin := normalizeHexAddress(config.AppConfig.AdminWalletAddress)
	if admin == "" || database.DB == nil {
		return false
	}
	var count int64
	_ = database.DB.Model(&models.Wallet{}).
		Where("user_id = ? AND LOWER(address) = ?", userID, admin).
		Count(&count).Error
	return count > 0
}

func (s *AccessService) GetUserAccess(userID uint) (*models.UserAccess, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	var access models.UserAccess
	if err := database.DB.Where("user_id = ?", userID).First(&access).Error; err != nil {
		return nil, err
	}
	return &access, nil
}

func (s *AccessService) CheckUserAccess(userID uint, now time.Time) (AccessCheck, error) {
	if s.IsAdminUser(userID) {
		return AccessCheck{Allowed: true, IsAdmin: true}, nil
	}

	access, err := s.GetUserAccess(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AccessCheck{Allowed: false, Reason: "未授权"}, nil
		}
		return AccessCheck{Allowed: false, Reason: "查询授权失败"}, err
	}

	if access.RevokedAt != nil {
		return AccessCheck{Allowed: false, Access: access, Reason: "已被停用"}, nil
	}
	if access.ActiveFrom != nil && now.Before(*access.ActiveFrom) {
		return AccessCheck{Allowed: false, Access: access, Reason: "未到生效时间"}, nil
	}
	if access.ActiveTo != nil && now.After(*access.ActiveTo) {
		return AccessCheck{Allowed: false, Access: access, Reason: "已过期"}, nil
	}
	return AccessCheck{Allowed: true, Access: access}, nil
}

type CreateAuthCodeInput struct {
	ActiveFrom     *time.Time
	ActiveTo       *time.Time
	MaxWallets     int
	MaxActiveTasks int
	MaxRedemptions int
	Note           string
}

func generateAuthCode() (string, error) {
	// 10 bytes -> 16 base32 chars (no padding), safe for manual input.
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return strings.ToUpper(code), nil
}

func (s *AccessService) CreateAuthCode(createdByUserID uint, in CreateAuthCodeInput) (*models.AuthCode, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	if in.ActiveFrom != nil && in.ActiveTo != nil && in.ActiveFrom.After(*in.ActiveTo) {
		return nil, errors.New("invalid time range")
	}
	if in.MaxRedemptions <= 0 {
		in.MaxRedemptions = 1
	}
	if in.MaxWallets < 0 {
		in.MaxWallets = 0
	}
	if in.MaxActiveTasks < 0 {
		in.MaxActiveTasks = 0
	}

	var lastErr error
	for i := 0; i < 5; i++ {
		code, err := generateAuthCode()
		if err != nil {
			return nil, fmt.Errorf("generate code failed: %w", err)
		}
		ac := &models.AuthCode{
			Code:            code,
			CreatedByUserID: createdByUserID,
			Note:            strings.TrimSpace(in.Note),
			ActiveFrom:      in.ActiveFrom,
			ActiveTo:        in.ActiveTo,
			MaxRedemptions:  in.MaxRedemptions,
			MaxWallets:      in.MaxWallets,
			MaxActiveTasks:  in.MaxActiveTasks,
		}
		if err := database.DB.Create(ac).Error; err != nil {
			lastErr = err
			continue
		}
		return ac, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("create auth code failed: %w", lastErr)
	}
	return nil, errors.New("create auth code failed")
}

// GetAuthCode 获取授权码详情
func (s *AccessService) GetAuthCode(codeID uint) (*models.AuthCode, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	var code models.AuthCode
	if err := database.DB.First(&code, codeID).Error; err != nil {
		return nil, err
	}
	return &code, nil
}

// UpdateAuthCodeInput 更新授权码的输入参数
type UpdateAuthCodeInput struct {
	ActiveTo       *time.Time
	MaxWallets     *int
	MaxActiveTasks *int
	MaxRedemptions *int
	Note           *string
}

// UpdateAuthCode 更新授权码
func (s *AccessService) UpdateAuthCode(codeID uint, in UpdateAuthCodeInput) (*models.AuthCode, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}

	updates := map[string]interface{}{}
	if in.ActiveTo != nil {
		updates["active_to"] = in.ActiveTo
	}
	if in.MaxWallets != nil {
		updates["max_wallets"] = *in.MaxWallets
	}
	if in.MaxActiveTasks != nil {
		updates["max_active_tasks"] = *in.MaxActiveTasks
	}
	if in.MaxRedemptions != nil {
		updates["max_redemptions"] = *in.MaxRedemptions
	}
	if in.Note != nil {
		updates["note"] = strings.TrimSpace(*in.Note)
	}

	if len(updates) == 0 {
		return s.GetAuthCode(codeID)
	}

	if err := database.DB.Model(&models.AuthCode{}).Where("id = ?", codeID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetAuthCode(codeID)
}

// DisableAuthCode 停用授权码
func (s *AccessService) DisableAuthCode(codeID uint) error {
	if database.DB == nil {
		return errors.New("database not initialized")
	}
	now := time.Now()
	return database.DB.Model(&models.AuthCode{}).Where("id = ?", codeID).Update("disabled_at", &now).Error
}

// EnableAuthCode 启用授权码
func (s *AccessService) EnableAuthCode(codeID uint) error {
	if database.DB == nil {
		return errors.New("database not initialized")
	}
	return database.DB.Model(&models.AuthCode{}).Where("id = ?", codeID).Update("disabled_at", nil).Error
}

func (s *AccessService) RedeemAuthCode(userID uint, rawCode string) (*models.UserAccess, *models.AuthCode, error) {
	if database.DB == nil {
		return nil, nil, errors.New("database not initialized")
	}
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if code == "" {
		return nil, nil, errors.New("empty code")
	}

	now := time.Now()
	var auth models.AuthCode
	var access models.UserAccess

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("code = ?", code).
			First(&auth).Error; err != nil {
			return err
		}

		if auth.DisabledAt != nil {
			return errors.New("code disabled")
		}
		if auth.ActiveFrom != nil && now.Before(*auth.ActiveFrom) {
			return errors.New("code not active yet")
		}
		if auth.ActiveTo != nil && now.After(*auth.ActiveTo) {
			return errors.New("code expired")
		}
		if auth.MaxRedemptions > 0 && auth.RedeemedCount >= auth.MaxRedemptions {
			return errors.New("code exhausted")
		}

		auth.RedeemedCount++
		if err := tx.Model(&auth).Update("redeemed_count", auth.RedeemedCount).Error; err != nil {
			return err
		}

		err := tx.Where("user_id = ?", userID).First(&access).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		updates := map[string]interface{}{
			"granted_by_user_id": auth.CreatedByUserID,
			"granted_by_code_id": auth.ID,
			"active_from":        auth.ActiveFrom,
			"active_to":          auth.ActiveTo,
			"max_wallets":        auth.MaxWallets,
			"max_active_tasks":   auth.MaxActiveTasks,
			"revoked_at":         nil,
			"revoked_by_user_id": nil,
			"note":               strings.TrimSpace(auth.Note),
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			access = models.UserAccess{UserID: userID}
			if err := tx.Create(&access).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&access).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).First(&access).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &access, &auth, nil
}

func (s *AccessService) CountUserWallets(userID uint) (int64, error) {
	if database.DB == nil {
		return 0, errors.New("database not initialized")
	}
	var count int64
	if err := database.DB.Model(&models.Wallet{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *AccessService) CountUserActiveTasks(userID uint) (int64, error) {
	if database.DB == nil {
		return 0, errors.New("database not initialized")
	}
	var count int64
	if err := database.DB.Model(&models.StrategyTask{}).
		Where("user_id = ? AND status IN ?", userID, []models.StrategyStatus{
			models.StrategyStatusRunning,
			models.StrategyStatusWaiting,
			models.StrategyStatusStopping,
		}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *AccessService) ListUserAccesses(limit int) ([]models.UserAccess, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var accesses []models.UserAccess
	if err := database.DB.Preload("User").Order("updated_at DESC").Limit(limit).Find(&accesses).Error; err != nil {
		return nil, err
	}
	return accesses, nil
}

// ListUserAccessesPaged 分页获取用户权限列表
func (s *AccessService) ListUserAccessesPaged(page, pageSize int) ([]models.UserAccess, int64, error) {
	if database.DB == nil {
		return nil, 0, errors.New("database not initialized")
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 20 {
		pageSize = 10
	}

	var total int64
	if err := database.DB.Model(&models.UserAccess{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var accesses []models.UserAccess
	offset := (page - 1) * pageSize
	if err := database.DB.Preload("User").Order("updated_at DESC").Offset(offset).Limit(pageSize).Find(&accesses).Error; err != nil {
		return nil, 0, err
	}
	return accesses, total, nil
}

// SearchUserAccess 按用户名或ID搜索用户
func (s *AccessService) SearchUserAccess(query string) ([]models.UserAccess, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("查询条件不能为空")
	}

	var accesses []models.UserAccess

	// 如果是数字，按 Telegram ID 搜索
	if telegramID, err := strconv.ParseInt(query, 10, 64); err == nil {
		var user models.User
		if err := database.DB.Where("telegram_id = ?", telegramID).First(&user).Error; err == nil {
			var access models.UserAccess
			if err := database.DB.Preload("User").Where("user_id = ?", user.ID).First(&access).Error; err == nil {
				accesses = append(accesses, access)
			}
		}
		return accesses, nil
	}

	// 去掉 @ 符号
	if strings.HasPrefix(query, "@") {
		query = query[1:]
	}

	// 按用户名搜索
	var users []models.User
	if err := database.DB.Where("username LIKE ? OR first_name LIKE ?", "%"+query+"%", "%"+query+"%").Limit(10).Find(&users).Error; err != nil {
		return nil, err
	}

	for _, user := range users {
		var access models.UserAccess
		if err := database.DB.Preload("User").Where("user_id = ?", user.ID).First(&access).Error; err == nil {
			accesses = append(accesses, access)
		}
	}

	return accesses, nil
}

// CountTotalUsers 统计总用户数
func (s *AccessService) CountTotalUsers() (int64, error) {
	if database.DB == nil {
		return 0, errors.New("database not initialized")
	}
	var count int64
	if err := database.DB.Model(&models.UserAccess{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountActiveUsers 统计活跃用户数（未停用且未过期）
func (s *AccessService) CountActiveUsers() (int64, error) {
	if database.DB == nil {
		return 0, errors.New("database not initialized")
	}
	var count int64
	now := time.Now()
	if err := database.DB.Model(&models.UserAccess{}).
		Where("revoked_at IS NULL").
		Where("active_to IS NULL OR active_to > ?", now).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *AccessService) GetUserAccessWithUser(userID uint) (*models.UserAccess, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	var access models.UserAccess
	if err := database.DB.Preload("User").Where("user_id = ?", userID).First(&access).Error; err != nil {
		return nil, err
	}
	return &access, nil
}

type UpdateUserAccessInput struct {
	ActiveFrom     *time.Time
	ActiveTo       *time.Time
	MaxWallets     *int
	MaxActiveTasks *int
	Note           *string
}

func (s *AccessService) UpdateUserAccess(adminUserID uint, userID uint, in UpdateUserAccessInput) (*models.UserAccess, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	if in.ActiveFrom != nil && in.ActiveTo != nil && in.ActiveFrom.After(*in.ActiveTo) {
		return nil, errors.New("invalid time range")
	}

	updates := map[string]interface{}{
		"granted_by_user_id": adminUserID,
	}
	if in.ActiveFrom != nil {
		updates["active_from"] = in.ActiveFrom
	}
	if in.ActiveTo != nil {
		updates["active_to"] = in.ActiveTo
	}
	if in.MaxWallets != nil {
		updates["max_wallets"] = *in.MaxWallets
	}
	if in.MaxActiveTasks != nil {
		updates["max_active_tasks"] = *in.MaxActiveTasks
	}
	if in.Note != nil {
		updates["note"] = strings.TrimSpace(*in.Note)
	}

	if err := database.DB.Model(&models.UserAccess{}).Where("user_id = ?", userID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetUserAccessWithUser(userID)
}

func (s *AccessService) RevokeUserAccess(adminUserID uint, userID uint) error {
	if database.DB == nil {
		return errors.New("database not initialized")
	}
	now := time.Now()
	return database.DB.Model(&models.UserAccess{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"revoked_at":         &now,
			"revoked_by_user_id": adminUserID,
		}).Error
}

func (s *AccessService) RestoreUserAccess(adminUserID uint, userID uint) error {
	if database.DB == nil {
		return errors.New("database not initialized")
	}
	return database.DB.Model(&models.UserAccess{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"revoked_at":         nil,
			"revoked_by_user_id": nil,
			"granted_by_user_id": adminUserID,
		}).Error
}
