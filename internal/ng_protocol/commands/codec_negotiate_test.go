package commands

import (
	"testing"

	"karl/internal"
)

func TestNegotiateCodecs(t *testing.T) {
	offerer := []internal.CodecInfo{
		{PayloadType: 0, Name: "PCMU", ClockRate: 8000},
		{PayloadType: 8, Name: "PCMA", ClockRate: 8000},
		{PayloadType: 101, Name: "telephone-event", ClockRate: 8000},
	}
	answerer := []internal.CodecInfo{
		{PayloadType: 8, Name: "PCMA", ClockRate: 8000},
		{PayloadType: 96, Name: "opus", ClockRate: 48000},
		{PayloadType: 101, Name: "telephone-event", ClockRate: 8000},
	}

	result := negotiateCodecs(offerer, answerer)

	// Should be in offerer's order, only common codecs
	if len(result) != 2 {
		t.Fatalf("expected 2 codecs, got %d: %+v", len(result), result)
	}
	if result[0].Name != "PCMA" {
		t.Errorf("first codec should be PCMA (offerer's order), got %s", result[0].Name)
	}
	if result[1].Name != "telephone-event" {
		t.Errorf("second codec should be telephone-event, got %s", result[1].Name)
	}
	// Payload type should be from answerer
	if result[0].PayloadType != 8 {
		t.Errorf("payload type should be from answerer (8), got %d", result[0].PayloadType)
	}
}

func TestNegotiateCodecs_NoOverlap(t *testing.T) {
	offerer := []internal.CodecInfo{{PayloadType: 0, Name: "PCMU", ClockRate: 8000}}
	answerer := []internal.CodecInfo{{PayloadType: 96, Name: "opus", ClockRate: 48000}}

	result := negotiateCodecs(offerer, answerer)

	// Should fall back to answerer's full list
	if len(result) != 1 || result[0].Name != "opus" {
		t.Errorf("no overlap should return answerer's list, got %+v", result)
	}
}

func TestNegotiateCodecs_EmptyInput(t *testing.T) {
	codecs := []internal.CodecInfo{{PayloadType: 0, Name: "PCMU", ClockRate: 8000}}

	if r := negotiateCodecs(nil, codecs); len(r) != 1 {
		t.Error("nil offerer should return answerer")
	}
	if r := negotiateCodecs(codecs, nil); len(r) != 0 {
		t.Error("nil answerer should return nil")
	}
}
