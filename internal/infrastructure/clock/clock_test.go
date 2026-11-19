package clock

import (
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func TestSystemReportsLocalTime(t *testing.T) {
	var c shared.Clock = System{}
	before := time.Now()
	now := c.Now()
	after := time.Now()
	if now.Before(before) || now.After(after) {
		t.Fatalf("Now returned %v, outside [%v, %v]", now, before, after)
	}
	if now.Location() != time.Local {
		t.Errorf("Now returned %v in %v, want local time", now, now.Location())
	}
}
