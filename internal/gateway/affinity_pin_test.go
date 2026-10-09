package gateway

import (
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/provider"
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
}

func TestOfficialContextSurvivesHostReplacement(t *testing.T) {
	s := &Server{}
	facts := provider.TaskContext{StartedAt: time.Now().UTC(), Model: "bytedance/model", Resolution: "1080p", HasVideo: true, Known: true}
	s.PinOfficialContext("scope", facts)
	restored := &Server{affinity: s.affinity}
	if got := restored.OfficialContext("scope"); got != facts {
		t.Fatalf("context lost: %+v", got)
	}
	pin := s.affinity["official_context:v1:scope"]
	if time.Until(pin.until) < officialPinTTL-time.Minute {
		t.Fatal("wrong context TTL")
	}
	if restored.OfficialContext("other").Known {
		t.Fatal("cross-task context leak")
	}
}

func TestOfficialContextSurvivesRedisHostReplacement(t *testing.T) {
	c := settlementRedis(t)
	s := &Server{Live: c}
	facts := provider.TaskContext{StartedAt: time.Now().UTC(), Model: "bytedance/model", Resolution: "480p", Known: true}
	s.PinOfficialContext("scope", facts)
	restored := &Server{Live: c}
	if got := restored.OfficialContext("scope"); got != facts {
		t.Fatalf("Redis lost billing facts: %+v", got)
	}
}
