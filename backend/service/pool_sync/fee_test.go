package pool_sync

import "testing"

func TestNormalizePoolSyncFeeDropsV4DynamicFeeFlag(t *testing.T) {
	t.Parallel()

	tier, pct, dynamic := normalizePoolSyncFee("v4", 0x800000, 838.8608)
	if !dynamic {
		t.Fatal("expected v4 dynamic fee flag to be marked dynamic")
	}
	if tier != 0 || pct != 0 {
		t.Fatalf("dynamic fee output = tier %d pct %.4f, want zeros", tier, pct)
	}
}

func TestNormalizePoolSyncFeeKeepsStaticV3Fee(t *testing.T) {
	t.Parallel()

	tier, pct, dynamic := normalizePoolSyncFee("v3", 2500, 0.25)
	if dynamic {
		t.Fatal("static v3 fee should not be dynamic")
	}
	if tier != 2500 || pct != 0.25 {
		t.Fatalf("static fee output = tier %d pct %.4f, want 2500/0.25", tier, pct)
	}
}
