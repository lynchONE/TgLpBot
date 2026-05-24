package web_server

import "testing"

func TestNormalizeSmartMoneyWalletSourceScope(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		want   string
		wantOK bool
	}{
		{name: "empty", raw: "", want: "", wantOK: true},
		{name: "all", raw: "all", want: "", wantOK: true},
		{name: "manual", raw: "manual", want: "manual", wantOK: true},
		{name: "contract alias", raw: "contract", want: "contract_interaction", wantOK: true},
		{name: "contract interaction", raw: "contract_interaction", want: "contract_interaction", wantOK: true},
		{name: "invalid", raw: "unknown", want: "", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := normalizeSmartMoneyWalletSourceScope(tc.raw)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("normalizeSmartMoneyWalletSourceScope(%q) = %q, %v; want %q, %v", tc.raw, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
