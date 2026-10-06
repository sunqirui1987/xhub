package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// verifyChain signs in as each verification account and checks the listing
// scope. It talks to a gateway that is already running. The gateway does not
// call this, and a deployment that never runs the seed command has none of
// these accounts.
func verifyChain(base string) error {
	base = strings.TrimRight(base, "/")
	admin, err := login(base, "admin@xhub.local", "admin-pass-1234")
	if err != nil {
		return fmt.Errorf("platform administrator: %w", err)
	}
	orgA, err := login(base, "org-a-admin@xhub.local", demoPassword)
	if err != nil {
		return err
	}
	teamA1, err := login(base, "team-a1-admin@xhub.local", demoPassword)
	if err != nil {
		return err
	}
	memberA1, err := login(base, "member-a1@xhub.local", demoPassword)
	if err != nil {
		return err
	}
	teamA2, err := login(base, "team-a2-admin@xhub.local", demoPassword)
	if err != nil {
		return err
	}
	orgB, err := login(base, "org-b-admin@xhub.local", demoPassword)
	if err != nil {
		return err
	}
	memberB1, err := login(base, "member-b1@xhub.local", demoPassword)
	if err != nil {
		return err
	}
	outsider, err := login(base, "outsider@xhub.local", demoPassword)
	if err != nil {
		return err
	}

	teams := map[string]string{}
	for _, who := range []session{admin} {
		names, err := who.names("/team/list", "", "team_alias")
		if err != nil {
			return err
		}
		for _, name := range names {
			teams[name] = name
		}
	}
	for _, want := range []string{"甲一组", "甲二组", "乙一组"} {
		if _, ok := teams[want]; !ok {
			return fmt.Errorf("platform administrator does not see team %s", want)
		}
	}

	checks := []struct {
		name    string
		who     session
		path    string
		wrap    string
		field   string
		present []string
		absent  []string
	}{
		{"org A teams", orgA, "/team/list", "", "team_alias", []string{"甲一组", "甲二组"}, []string{"乙一组"}},
		{"team A1 teams", teamA1, "/team/list", "", "team_alias", []string{"甲一组"}, []string{"甲二组", "乙一组"}},
		{"team A2 teams", teamA2, "/team/list", "", "team_alias", []string{"甲二组"}, []string{"甲一组", "乙一组"}},
		{"org B teams", orgB, "/team/list", "", "team_alias", []string{"乙一组"}, []string{"甲一组", "甲二组"}},
		{"member A1 teams", memberA1, "/team/list", "", "team_alias", []string{"甲一组"}, []string{"甲二组", "乙一组"}},
		{"member B1 teams", memberB1, "/team/list", "", "team_alias", []string{"乙一组"}, []string{"甲一组"}},
		{"outsider teams", outsider, "/team/list", "", "team_alias", nil, []string{"甲一组", "乙一组"}},
		{"org A projects", orgA, "/project/list", "projects", "project_alias", []string{"甲一组演示项目"}, []string{"乙一组演示项目"}},
		{"member A1 projects", memberA1, "/project/list", "projects", "project_alias", []string{"甲一组演示项目"}, []string{"乙一组演示项目"}},
		{"team A2 projects", teamA2, "/project/list", "projects", "project_alias", nil, []string{"甲一组演示项目", "乙一组演示项目"}},
		{"org A users", orgA, "/user/list", "users", "user_email", []string{"member-a1@xhub.local", "team-a2-admin@xhub.local"}, []string{"member-b1@xhub.local", "outsider@xhub.local"}},
		{"team A1 users", teamA1, "/user/list", "users", "user_email", []string{"member-a1@xhub.local", "org-a-admin@xhub.local"}, []string{"team-a2-admin@xhub.local", "member-b1@xhub.local"}},
		{"member A1 users", memberA1, "/user/list", "users", "user_email", []string{"member-a1@xhub.local"}, []string{"team-a1-admin@xhub.local", "member-b1@xhub.local"}},
		{"outsider users", outsider, "/user/list", "users", "user_email", []string{"outsider@xhub.local"}, []string{"member-a1@xhub.local"}},
	}
	for _, check := range checks {
		got, err := check.who.names(check.path, check.wrap, check.field)
		if err != nil {
			// Only the check name is logged. The error can carry a response
			// body, and a login body holds a session token, which logx does
			// not recognise: it redacts bearer and sk- values, not sess-.
			logx.Error("seed verify %s: read failed", check.name)
			return fmt.Errorf("%s: %w", check.name, err)
		}
		if err := expectSet(check.name, got, check.present, check.absent); err != nil {
			logx.Error("seed verify %s: scope wrong", check.name)
			return err
		}
		logx.Debug("seed verify %s: ok", check.name)
	}

	orgs, err := orgA.names("/organization/list", "", "organization_alias")
	if err != nil {
		return err
	}
	if err := expectSet("org A organizations", orgs, []string{"组织甲"}, []string{"组织乙"}); err != nil {
		return err
	}
	return nil
}

func expectSet(name string, got, present, absent []string) error {
	have := map[string]bool{}
	for _, v := range got {
		have[v] = true
	}
	for _, want := range present {
		if !have[want] {
			return fmt.Errorf("%s missing %s, saw %s", name, want, strings.Join(got, ","))
		}
	}
	for _, ban := range absent {
		if have[ban] {
			return fmt.Errorf("%s leaked %s, saw %s", name, ban, strings.Join(got, ","))
		}
	}
	return nil
}

type session struct {
	base  string
	email string
	token string
}

func login(base, email, password string) (session, error) {
	body, _ := json.Marshal(map[string]string{"username": email, "password": password})
	res, err := http.Post(base+"/v2/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return session{}, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return session{}, fmt.Errorf("login %s: %d %s", email, res.StatusCode, raw)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return session{}, err
	}
	token, _ := parsed["token"].(string)
	if token == "" {
		token, _ = parsed["key"].(string)
	}
	if token == "" {
		return session{}, fmt.Errorf("login %s: no token", email)
	}
	return session{base: base, email: email, token: token}, nil
}

func (s session) names(path, wrap, field string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, s.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s as %s: %d %s", path, s.email, res.StatusCode, raw)
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if wrap != "" {
		obj, _ := payload.(map[string]any)
		payload = obj[wrap]
	}
	rows, _ := payload.([]any)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		obj, _ := row.(map[string]any)
		if v, ok := obj[field].(string); ok && v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}
