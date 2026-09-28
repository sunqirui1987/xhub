package gateway

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/store"
)

func TestDailyActivityHTTPReadsInsertedSpendLog(t *testing.T) {
	cfg, err := config.Load(configPath(t))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	id := "activity-probe-" + time.Now().UTC().Format("20060102150405.000000000")
	start := time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)
	if err := st.InsertSpendLog(id, "chat", "activity-probe-model", "probe-hash", 11, 7, sql.NullFloat64{Float64: 3.5, Valid: true}, start, start.Add(time.Second), false, "success"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = st.DB.Exec(`DELETE FROM spend_logs WHERE request_id = $1`, id)
	})

	gw := New(cfg, st)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	status, body := authed(t, srv.URL, cfg.GeneralSettings.MasterKey, http.MethodGet, "/user/daily/activity/aggregated?start_date=2026-09-27&end_date=2026-09-27", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d %s", status, trim(body))
	}
	var parsed struct {
		Results []struct {
			Date      string `json:"date"`
			Breakdown struct {
				Models map[string]struct {
					Metrics struct {
						Spend       float64 `json:"spend"`
						APIRequests int     `json:"api_requests"`
					} `json:"metrics"`
				} `json:"models"`
				ModelGroups map[string]json.RawMessage `json:"model_groups"`
			} `json:"breakdown"`
		} `json:"results"`
		Metadata struct {
			TotalSpend      float64 `json:"total_spend"`
			TotalAPI        int     `json:"total_api_requests"`
			TotalSuccessful int     `json:"total_successful_requests"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	var probeSpend float64
	var probeRequests int
	var foundProbe bool
	for _, day := range parsed.Results {
		model, ok := day.Breakdown.Models["activity-probe-model"]
		if !ok {
			continue
		}
		foundProbe = true
		probeSpend = model.Metrics.Spend
		probeRequests = model.Metrics.APIRequests
		if _, grouped := day.Breakdown.ModelGroups["activity-probe-model"]; !grouped {
			t.Fatal("probe model missing from model_groups")
		}
	}
	if !foundProbe {
		t.Fatalf("probe model missing from %s", trim(body))
	}
	if probeRequests < 1 || probeSpend < 3.5 {
		t.Fatalf("probe metrics spend=%v requests=%d", probeSpend, probeRequests)
	}
	if parsed.Metadata.TotalAPI < 1 || parsed.Metadata.TotalSpend < 3.5 || parsed.Metadata.TotalSuccessful < 1 {
		t.Fatalf("metadata: %+v", parsed.Metadata)
	}

	gwStatus, gwBody := authed(t, srv.URL, cfg.GeneralSettings.MasterKey, http.MethodGet, "/gateway/daily/activity?start_date=2026-09-27&end_date=2026-09-27", nil)
	if gwStatus != http.StatusOK {
		t.Fatalf("gateway status %d %s", gwStatus, trim(gwBody))
	}
	var counts struct {
		TotalSuccessful int `json:"total_successful_requests"`
		ByRoute         []struct {
			Route      string `json:"route"`
			Successful int    `json:"successful_requests"`
		} `json:"by_route"`
	}
	if err := json.Unmarshal(gwBody, &counts); err != nil {
		t.Fatal(err)
	}
	if counts.TotalSuccessful < 1 {
		t.Fatalf("gateway counts: %s", trim(gwBody))
	}
	foundRoute := false
	for _, route := range counts.ByRoute {
		if route.Route == "/chat/completions" && route.Successful >= 1 {
			foundRoute = true
		}
	}
	if !foundRoute {
		t.Fatalf("chat route missing: %s", trim(gwBody))
	}
}
