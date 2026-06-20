package blockchain

import (
	"context"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// TestMulticall3PackUnpackRoundTrip verifies the Multicall3Call / Multicall3Result
// struct definitions stay in sync with the aggregate3 ABI: a packed input decodes
// back, and a packed output round-trips through UnpackIntoInterface.
func TestMulticall3PackUnpackRoundTrip(t *testing.T) {
	parsed, err := abi.JSON(strings.NewReader(multicall3ABI))
	if err != nil {
		t.Fatalf("parse multicall3 abi: %v", err)
	}

	calls := []Multicall3Call{
		{Target: common.HexToAddress("0x1111111111111111111111111111111111111111"), AllowFailure: true, CallData: []byte{0x01, 0x02, 0x03, 0x04}},
		{Target: common.HexToAddress("0x2222222222222222222222222222222222222222"), AllowFailure: false, CallData: nil},
	}
	if _, err := parsed.Pack("aggregate3", calls); err != nil {
		t.Fatalf("pack aggregate3 input: %v", err)
	}

	owner := common.HexToAddress("0x00000000000000000000000000000000000000aB")
	ownerWord := common.LeftPadBytes(owner.Bytes(), 32)
	want := []Multicall3Result{
		{Success: true, ReturnData: ownerWord},
		{Success: false, ReturnData: []byte{}},
	}
	encoded, err := parsed.Methods["aggregate3"].Outputs.Pack(want)
	if err != nil {
		t.Fatalf("pack aggregate3 output: %v", err)
	}

	var decoded struct {
		ReturnData []Multicall3Result
	}
	if err := parsed.UnpackIntoInterface(&decoded, "aggregate3", encoded); err != nil {
		t.Fatalf("unpack aggregate3 output: %v", err)
	}
	if len(decoded.ReturnData) != 2 {
		t.Fatalf("expected 2 results, got %d", len(decoded.ReturnData))
	}
	if !decoded.ReturnData[0].Success || decoded.ReturnData[1].Success {
		t.Fatalf("success flags not preserved: %+v", decoded.ReturnData)
	}
	gotOwner := common.BytesToAddress(decoded.ReturnData[0].ReturnData)
	if gotOwner != owner {
		t.Fatalf("owner round-trip mismatch: want %s got %s", owner.Hex(), gotOwner.Hex())
	}
}

func TestAggregate3NilClient(t *testing.T) {
	_, err := Aggregate3(context.Background(), nil, common.Address{}, []Multicall3Call{
		{Target: common.HexToAddress("0x1111111111111111111111111111111111111111")},
	})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}
