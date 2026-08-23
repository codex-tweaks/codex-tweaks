package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCodableTimeRoundTripsRFC3339Nanoseconds(t *testing.T) {
	value := NewCodableTime(time.Date(2026, 8, 23, 10, 11, 12, 987654321, time.UTC))
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `"2026-08-23T10:11:12.987654321Z"` {
		t.Fatalf("unexpected persisted time: %s", encoded)
	}

	var decoded CodableTime
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Equal(value.Time) {
		t.Fatalf("round trip changed timestamp: %s", decoded.Time)
	}
}
