package redis

import (
	"reflect"
	"testing"
)

func TestParseStreamReply(t *testing.T) {
	// XADD events * tag first tag second stores both pairs. A map-based decoder
	// keeps one; the panel must show what Redis holds.
	reply := []any{
		[]any{"1-0", []any{"tag", "first", "tag", "second", "kind", "deploy"}},
		[]any{"2-0", []any{}},
	}

	got, err := parseStreamReply(reply)
	if err != nil {
		t.Fatalf("parseStreamReply() unexpected error: %v", err)
	}
	want := []RawEntry{
		{ID: "1-0", Fields: []RawPair{{"tag", "first"}, {"tag", "second"}, {"kind", "deploy"}}},
		{ID: "2-0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseStreamReply() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseStreamReplyRefusesAShapeItDoesNotKnow(t *testing.T) {
	if _, err := parseStreamReply([]any{"not an entry"}); err == nil {
		t.Error("an unexpected reply shape must be an error, not an empty entry")
	}
}
