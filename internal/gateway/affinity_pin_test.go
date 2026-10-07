package gateway

import (
	"testing"
	"time"
)

func TestOfficialTaskPinLastsSevenDays(t *testing.T) {
	if officialPinTTL != 7*24*time.Hour {
		t.Fatalf("official pin ttl %s", officialPinTTL)
	}
	s := &Server{}
	s.PinOfficial("cgt-1", "base|model")
	if got := s.OfficialDeployment("cgt-1"); got != "base|model" {
		t.Fatalf("pin %q", got)
	}
	if s.OfficialBilled("cgt-1") {
		t.Fatal("new task was already billed")
	}
	s.MarkOfficialBilled("cgt-1")
	if !s.OfficialBilled("cgt-1") {
		t.Fatal("billed flag was not stored")
	}
}
