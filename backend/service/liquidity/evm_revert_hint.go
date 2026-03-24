package liquidity

import "strings"

const permit2AllowanceIsFixedAtInfinitySelector = "0x3f68539a"
const cannotUpdateEmptyPositionSelector = "0xaefeb924"

func evmRevertHint(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()

	// Permit2AllowanceIsFixedAtInfinity()
	if strings.Contains(msg, permit2AllowanceIsFixedAtInfinitySelector) {
		return "revert 0x3f68539a=Permit2AllowanceIsFixedAtInfinity(): 通常是 Permit2.approve 在估算 gas 阶段回滚（BscScan 看不到交易）。多半是 ZapSimple 合约老版本没做 try/catch；请重新部署最新 ZapSimple 并更新 ZAP_V4_ADDRESS"
	}

	if strings.Contains(msg, cannotUpdateEmptyPositionSelector) {
		return "revert 0xaefeb924=CannotUpdateEmptyPosition(): V4 最终算出的 liquidity=0。常见原因是区间内需要先 swap 成双边，但 OKX swap 准备失败后仍按单边 zero-swap 继续"
	}

	return ""
}
