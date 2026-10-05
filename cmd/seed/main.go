// Command seed loads a verification tenant into a database that already has
// the schema. It is not part of gateway startup. A normal start still creates
// only the administrator named in the config, and nothing else.
//
//	go run ./cmd/seed -config configs/config.yaml
//	go run ./cmd/seed -config configs/config.yaml -verify -gateway http://127.0.0.1:4000
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

const demoPassword = "demo-pass-1234"

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "gateway config, used only for the database URL")
	verify := flag.Bool("verify", false, "after loading, sign in to a running gateway and check each tier")
	gateway := flag.String("gateway", "http://127.0.0.1:4000", "gateway used by -verify")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fail("config: %v", err)
	}
	ctx := context.Background()
	db, err := iam.Open(ctx, cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		fail("database: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		fail("migrate: %v", err)
	}

	fmt.Println("this command writes a verification tenant. it is not run by gateway startup.")
	logx.Info("seed loading the verification tenant")
	report, err := seed(ctx, db)
	if err != nil {
		logx.Error("seed: %v", err)
		fail("seed: %v", err)
	}
	fmt.Print(report)
	if !*verify {
		logx.Info("seed loaded; verification not requested")
		fmt.Println("verification was not run. pass -verify while the gateway is listening.")
		return
	}
	if err := verifyChain(*gateway); err != nil {
		logx.Error("seed verify: %v", err)
		fail("verify: %v", err)
	}
	logx.Info("seed verification passed for every tier")
	fmt.Println("verification passed for every tier.")
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

type seeded struct {
	orgA, orgB       *iam.Organization
	teamA1, teamA2   *iam.Team
	teamB1           *iam.Team
	projectA1        *iam.Project
	projectB1        *iam.Project
	orgAdminA        *iam.User
	teamAdminA1      *iam.User
	memberA1         *iam.User
	teamAdminA2      *iam.User
	orgAdminB        *iam.User
	memberB1         *iam.User
	outsider         *iam.User
	personalKeyPlain string
	serviceKeyPlain  string
}

func seed(ctx context.Context, db *iam.DB) (string, error) {
	by := iam.Actor{Kind: "seed", ID: "seed"}
	var b strings.Builder
	s := &seeded{}

	var err error
	s.orgA, err = ensureOrg(ctx, db, by, "组织甲")
	if err != nil {
		return "", err
	}
	s.orgB, err = ensureOrg(ctx, db, by, "组织乙")
	if err != nil {
		return "", err
	}
	s.orgAdminA, err = ensureUser(ctx, db, by, "org-a-admin@xhub.local", "组织甲管理员")
	if err != nil {
		return "", err
	}
	s.teamAdminA1, err = ensureUser(ctx, db, by, "team-a1-admin@xhub.local", "甲一组管理员")
	if err != nil {
		return "", err
	}
	s.memberA1, err = ensureUser(ctx, db, by, "member-a1@xhub.local", "甲一组成员")
	if err != nil {
		return "", err
	}
	s.teamAdminA2, err = ensureUser(ctx, db, by, "team-a2-admin@xhub.local", "甲二组管理员")
	if err != nil {
		return "", err
	}
	s.orgAdminB, err = ensureUser(ctx, db, by, "org-b-admin@xhub.local", "组织乙管理员")
	if err != nil {
		return "", err
	}
	s.memberB1, err = ensureUser(ctx, db, by, "member-b1@xhub.local", "乙一组成员")
	if err != nil {
		return "", err
	}
	s.outsider, err = ensureUser(ctx, db, by, "outsider@xhub.local", "无团队用户")
	if err != nil {
		return "", err
	}

	s.teamA1, err = ensureTeam(ctx, db, by, s.orgA.ID, "甲一组", "组织甲的第一组", s.teamAdminA1.ID)
	if err != nil {
		return "", err
	}
	s.teamA2, err = ensureTeam(ctx, db, by, s.orgA.ID, "甲二组", "组织甲的第二组", s.teamAdminA2.ID)
	if err != nil {
		return "", err
	}
	s.teamB1, err = ensureTeam(ctx, db, by, s.orgB.ID, "乙一组", "组织乙的第一组", s.orgAdminB.ID)
	if err != nil {
		return "", err
	}
	if err := ensureMember(ctx, db, by, s.teamA1.ID, s.orgAdminA.Email, iam.TeamMember); err != nil {
		return "", err
	}
	if err := ensureMember(ctx, db, by, s.teamA1.ID, s.memberA1.Email, iam.TeamMember); err != nil {
		return "", err
	}
	if err := ensureMember(ctx, db, by, s.teamB1.ID, s.memberB1.Email, iam.TeamMember); err != nil {
		return "", err
	}
	if err := ensureOrgAdmin(ctx, db, by, s.orgA.ID, s.orgAdminA.Email); err != nil {
		return "", err
	}
	if err := ensureOrgAdmin(ctx, db, by, s.orgB.ID, s.orgAdminB.Email); err != nil {
		return "", err
	}

	s.projectA1, err = ensureProject(ctx, db, by, s.teamA1.ID, "甲一组演示项目")
	if err != nil {
		return "", err
	}
	s.projectB1, err = ensureProject(ctx, db, by, s.teamB1.ID, "乙一组演示项目")
	if err != nil {
		return "", err
	}

	keyBy := iam.Actor{Kind: "seed", ID: s.teamAdminA1.ID}
	personal, personalPlain, err := ensureKey(ctx, db, keyBy, iam.KeyInput{
		OwnerType: iam.OwnerPersonal, UserID: s.memberA1.ID, TeamID: s.teamA1.ID, Name: "甲一组成员个人密钥",
	})
	if err != nil {
		return "", err
	}
	service, servicePlain, err := ensureKey(ctx, db, keyBy, iam.KeyInput{
		OwnerType: iam.OwnerService, TeamID: s.teamA1.ID, ProjectID: s.projectA1.ID, Name: "甲一组服务密钥",
	})
	if err != nil {
		return "", err
	}
	s.personalKeyPlain = personalPlain
	s.serviceKeyPlain = servicePlain

	if err := ensureUsage(ctx, db, s, personal.ID, service.ID); err != nil {
		return "", err
	}

	fmt.Fprintf(&b, "verification tenant is loaded. passwords for every demo account: %s\n", demoPassword)
	fmt.Fprintf(&b, "platform administrator stays the config account (admin@xhub.local), created by gateway startup, not by this command.\n")
	fmt.Fprintf(&b, "组织甲 %s\n  甲一组 %s  项目 %s\n  甲二组 %s\n", s.orgA.ID, s.teamA1.ID, s.projectA1.ID, s.teamA2.ID)
	fmt.Fprintf(&b, "组织乙 %s\n  乙一组 %s  项目 %s\n", s.orgB.ID, s.teamB1.ID, s.projectB1.ID)
	fmt.Fprintf(&b, "accounts:\n")
	for _, line := range []struct{ email, who string }{
		{s.orgAdminA.Email, "组织甲管理员，同时是甲一组的普通成员"},
		{s.teamAdminA1.Email, "甲一组的团队管理员"},
		{s.memberA1.Email, "甲一组的普通成员"},
		{s.teamAdminA2.Email, "甲二组的团队管理员"},
		{s.orgAdminB.Email, "组织乙管理员，同时是乙一组的团队管理员"},
		{s.memberB1.Email, "乙一组的普通成员"},
		{s.outsider.Email, "不属于任何团队"},
	} {
		fmt.Fprintf(&b, "  %s  %s\n", line.email, line.who)
	}
	if personalPlain != "" {
		fmt.Fprintf(&b, "personal key for %s: %s\n", s.memberA1.Email, personalPlain)
	}
	if servicePlain != "" {
		fmt.Fprintf(&b, "service key for 甲一组: %s\n", servicePlain)
	}
	return b.String(), nil
}

func ensureOrg(ctx context.Context, db *iam.DB, by iam.Actor, name string) (*iam.Organization, error) {
	rows, err := db.ListOrgs(ctx, "")
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Name == name {
			return &rows[i], nil
		}
	}
	return db.CreateOrg(ctx, by, name, nil)
}

func ensureUser(ctx context.Context, db *iam.DB, by iam.Actor, email, name string) (*iam.User, error) {
	rows, err := db.ListUsers(ctx, email, 20, 0)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if strings.EqualFold(rows[i].Email, email) {
			return &rows[i], nil
		}
	}
	return db.CreateUser(ctx, by, iam.UserInput{
		Email: email, Name: name, Password: demoPassword, Role: iam.RoleUser,
	})
}

func ensureTeam(ctx context.Context, db *iam.DB, by iam.Actor, orgID, name, description, adminID string) (*iam.Team, error) {
	rows, err := db.ListTeams(ctx, "", orgID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Name == name {
			return &rows[i].Team, nil
		}
	}
	return db.CreateTeam(ctx, by, iam.TeamInput{
		OrganizationID: orgID, Name: name, Description: description, AdminUserID: adminID,
	})
}

func ensureMember(ctx context.Context, db *iam.DB, by iam.Actor, teamID, email, role string) error {
	_, err := db.AddMember(ctx, by, teamID, email, role)
	if err == nil || errors.Is(err, iam.ErrConflict) {
		return nil
	}
	return err
}

func ensureOrgAdmin(ctx context.Context, db *iam.DB, by iam.Actor, orgID, email string) error {
	_, err := db.AddOrgAdmin(ctx, by, orgID, email)
	return err
}

func ensureProject(ctx context.Context, db *iam.DB, by iam.Actor, teamID, name string) (*iam.Project, error) {
	rows, err := db.ListProjects(ctx, []string{teamID})
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Name == name {
			return &rows[i], nil
		}
	}
	return db.CreateProject(ctx, by, iam.ProjectInput{TeamID: teamID, Name: name})
}

func ensureKey(ctx context.Context, db *iam.DB, by iam.Actor, in iam.KeyInput) (*iam.Key, string, error) {
	rows, err := db.ListKeys(ctx, iam.KeyFilter{TeamID: in.TeamID})
	if err != nil {
		return nil, "", err
	}
	for i := range rows {
		if rows[i].Name == in.Name && rows[i].TeamID == in.TeamID {
			return &rows[i], "", nil
		}
	}
	return db.CreateKey(ctx, by, in)
}

func ensureUsage(ctx context.Context, db *iam.DB, s *seeded, personalKeyID, serviceKeyID string) error {
	now := time.Now().UTC()
	records := []iam.UsageRecord{
		usageEvent("seed-member-a1", now, personalKeyID, iam.OwnerPersonal, s.memberA1.ID, s.teamA1, s.projectA1, s.orgA.ID, 1.25),
		usageEvent("seed-team-admin-a1", now, serviceKeyID, iam.OwnerService, s.teamAdminA1.ID, s.teamA1, s.projectA1, s.orgA.ID, 2.50),
		usageEvent("seed-member-b1", now, "", iam.OwnerPersonal, s.memberB1.ID, s.teamB1, s.projectB1, s.orgB.ID, 3.75),
	}
	return db.RecordUsage(ctx, records)
}

func usageEvent(id string, ts time.Time, keyID, owner, userID string, team *iam.Team, project *iam.Project, orgID string, cost float64) iam.UsageRecord {
	return iam.UsageRecord{
		RequestID: id, TS: ts, KeyID: keyID, OwnerType: owner, UserID: userID,
		TeamID: team.ID, ProjectID: project.ID, OrganizationID: orgID,
		Model: "demo-model", CallType: "chat", Status: "success",
		PromptTokens: 20, CompletionTokens: 10, Cost: cost, DurationMS: 40,
		RequestBody: `{"seed":true}`, ResponseBody: `{"ok":true}`,
	}
}
