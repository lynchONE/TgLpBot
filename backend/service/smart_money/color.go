package smart_money

import (
	"hash/crc32"
	"strings"
)

var walletPalette = []string{
	"#7F77DD", // purple
	"#1D9E75", // green
	"#D85A30", // orange-red
	"#BA7517", // amber
	"#185FA5", // blue
	"#A32D2D", // red
	"#3B6D11", // dark green
	"#993556", // pink
}

func WalletColor(address string) string {
	h := crc32.ChecksumIEEE([]byte(strings.ToLower(address)))
	return walletPalette[h%uint32(len(walletPalette))]
}
