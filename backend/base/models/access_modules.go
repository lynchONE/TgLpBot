package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	AccessModuleHotPools     = "hot_pools"
	AccessModuleGMGNKline    = "gmgn_kline"
	AccessModulePositions    = "positions"
	AccessModuleAssets       = "assets"
	AccessModuleSmartMoney   = "smart_money"
	AccessModuleSwap         = "swap"
	AccessModuleWalletManage = "wallet_manage"
	AccessModuleGlobalConfig = "global_config"
	AccessModuleCreatePool   = "create_pool"
	AccessModuleAdminPanel   = "admin_panel"
)

var ErrAccessModulesNotConfigured = errors.New("access modules not configured")

type AccessModule struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Group     string `json:"group"`
	Grantable bool   `json:"grantable"`
}

var accessModuleCatalog = []AccessModule{
	{Key: AccessModuleHotPools, Label: "热门池子", Group: "行情", Grantable: true},
	{Key: AccessModuleGMGNKline, Label: "K线", Group: "行情", Grantable: true},
	{Key: AccessModulePositions, Label: "仓位", Group: "交易", Grantable: true},
	{Key: AccessModuleCreatePool, Label: "创建池子", Group: "交易", Grantable: true},
	{Key: AccessModuleSwap, Label: "一键兑换", Group: "交易", Grantable: true},
	{Key: AccessModuleAssets, Label: "我的资产", Group: "资产", Grantable: true},
	{Key: AccessModuleWalletManage, Label: "钱包管理", Group: "资产", Grantable: true},
	{Key: AccessModuleGlobalConfig, Label: "全局配置", Group: "设置", Grantable: true},
	{Key: AccessModuleSmartMoney, Label: "聪明钱", Group: "分析", Grantable: true},
	{Key: AccessModuleAdminPanel, Label: "管理员", Group: "管理", Grantable: false},
}

func AccessModuleCatalog() []AccessModule {
	out := make([]AccessModule, len(accessModuleCatalog))
	copy(out, accessModuleCatalog)
	return out
}

func AccessModuleGrantableCatalog() []AccessModule {
	out := make([]AccessModule, 0, len(accessModuleCatalog))
	for _, item := range accessModuleCatalog {
		if item.Grantable {
			out = append(out, item)
		}
	}
	return out
}

func DefaultAccessModuleKeys() []string {
	items := AccessModuleGrantableCatalog()
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Key)
	}
	return out
}

func IsAccessModuleKey(key string) bool {
	_, ok := accessModuleByKey(strings.TrimSpace(key))
	return ok
}

func IsGrantableAccessModuleKey(key string) bool {
	item, ok := accessModuleByKey(strings.TrimSpace(key))
	return ok && item.Grantable
}

func NormalizeAccessModuleKeys(keys []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(keys))
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			return nil, errors.New("empty module key")
		}
		item, ok := accessModuleByKey(key)
		if !ok {
			return nil, fmt.Errorf("unknown module key: %s", key)
		}
		if !item.Grantable {
			return nil, fmt.Errorf("module is not grantable: %s", key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out, nil
}

func AccessModuleKeysToJSON(keys []string) (string, error) {
	normalized, err := NormalizeAccessModuleKeys(keys)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func AccessModuleKeysFromJSON(raw string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || text == "null" {
		return nil, ErrAccessModulesNotConfigured
	}
	var keys []string
	if err := json.Unmarshal([]byte(text), &keys); err != nil {
		return nil, err
	}
	if keys == nil {
		return nil, ErrAccessModulesNotConfigured
	}
	return NormalizeAccessModuleKeys(keys)
}

func AccessModuleKeysContain(keys []string, key string) bool {
	needle := strings.TrimSpace(key)
	for _, item := range keys {
		if item == needle {
			return true
		}
	}
	return false
}

func accessModuleByKey(key string) (AccessModule, bool) {
	for _, item := range accessModuleCatalog {
		if item.Key == key {
			return item, true
		}
	}
	return AccessModule{}, false
}
