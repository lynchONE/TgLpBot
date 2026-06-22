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

func TestParseSmartMoneyPositiveFilter(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantValue  float64
		wantActive bool
		wantErr    bool
	}{
		{name: "empty", raw: "", wantValue: 0, wantActive: false},
		{name: "spaces", raw: "   ", wantValue: 0, wantActive: false},
		{name: "zero", raw: "0", wantValue: 0, wantActive: false},
		{name: "positive", raw: "12.5", wantValue: 12.5, wantActive: true},
		{name: "negative", raw: "-1", wantErr: true},
		{name: "invalid", raw: "abc", wantErr: true},
		{name: "nan", raw: "NaN", wantErr: true},
		{name: "inf", raw: "+Inf", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotValue, gotActive, err := parseSmartMoneyPositiveFilter(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSmartMoneyPositiveFilter(%q) error = nil; want error", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSmartMoneyPositiveFilter(%q) error = %v; want nil", tc.raw, err)
			}
			if gotValue != tc.wantValue || gotActive != tc.wantActive {
				t.Fatalf("parseSmartMoneyPositiveFilter(%q) = %v, %v; want %v, %v", tc.raw, gotValue, gotActive, tc.wantValue, tc.wantActive)
			}
		})
	}
}
