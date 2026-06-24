package web_server

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseAlphaAirdropsKeepsTimedItems(t *testing.T) {
	raw := json.RawMessage(`{"airdrops":[{"token":"NES","name":"Nesa","date":"2026-06-24","time":"20:00"},{"token":"BAD","date":"2026-06-24"}]}`)
	got, err := parseAlphaAirdrops(raw)
	if err != nil {
		t.Fatalf("parse alpha airdrops: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one timed airdrop, got %d", len(got))
	}
	if got[0].Token != "NES" {
		t.Fatalf("unexpected token: %s", got[0].Token)
	}
}

func TestAlphaReminderDueUsesTriggerWindow(t *testing.T) {
	notifyAt := time.Date(2026, 6, 24, 19, 57, 0, 0, time.Local)
	if !alphaReminderDue(notifyAt, notifyAt) {
		t.Fatalf("expected due at notify time")
	}
	if !alphaReminderDue(notifyAt.Add(60*time.Second), notifyAt) {
		t.Fatalf("expected due inside trigger window")
	}
	if alphaReminderDue(notifyAt.Add(-time.Second), notifyAt) {
		t.Fatalf("expected not due before notify time")
	}
	if alphaReminderDue(notifyAt.Add(2*time.Minute), notifyAt) {
		t.Fatalf("expected not due outside trigger window")
	}
}

func TestAlphaReminderKeyIncludesTokenNameAndTime(t *testing.T) {
	item := alphaAirdropItem{Token: "nes", Name: "Nesa"}
	eventTime := time.Date(2026, 6, 24, 20, 0, 0, 0, time.Local)
	if got := alphaReminderKey(item, eventTime); got != "NES|Nesa|2026-06-24T20:00" {
		t.Fatalf("unexpected key: %s", got)
	}
}

func TestEncodeAlphaReminderSentKeysCapsHistory(t *testing.T) {
	values := map[string]bool{}
	for i := 0; i < alphaReminderSentKeyLimit+5; i++ {
		values[time.Date(2026, 6, 24, 0, i, 0, 0, time.Local).Format(time.RFC3339)] = true
	}
	encoded := encodeAlphaReminderSentKeys(values)
	var keys []string
	if err := json.Unmarshal([]byte(encoded), &keys); err != nil {
		t.Fatalf("decode sent keys: %v", err)
	}
	if len(keys) != alphaReminderSentKeyLimit {
		t.Fatalf("expected capped keys, got %d", len(keys))
	}
}

func TestAlphaReminderCompletionUpdatesDisablesReminder(t *testing.T) {
	updates := alphaReminderCompletionUpdates(map[string]bool{"NES|Nesa|2026-06-24T20:00": true})
	if updates["alpha_airdrop_reminder_enabled"] != false {
		t.Fatalf("expected reminder to be disabled after notification, got %+v", updates)
	}
	if updates["alpha_airdrop_reminder_sent_keys"] == "" {
		t.Fatalf("expected sent keys to be persisted")
	}
}
