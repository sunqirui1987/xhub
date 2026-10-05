package main

import (
	"context"
	"flag"
	"os"
	_ "time/tzdata"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "LiteLLM-shaped YAML config")
	addr := flag.String("addr", ":4000", "listen address")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logx.Error("config: %v", err)
		os.Exit(1)
	}
	ctx := context.Background()
	// iam owns every identity table: users, teams, organizations, projects,
	// keys and the audit log. store keeps the framework tables — key-value
	// settings, UI sessions, the spend queue — which carry no permissions.
	db, err := iam.Open(ctx, cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		logx.Error("iam: %v", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		logx.Error("iam migrate: %v", err)
		os.Exit(1)
	}
	st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		logx.Error("store: %v", err)
		os.Exit(1)
	}
	srv := gateway.New(cfg, st, db)
	logx.Info("xhub listening on %s", *addr)
	// 数据面和管理面都挂在这一个 Gin 引擎上。只留数据面会让 /key/generate 变成 404。
	if err := srv.Run(*addr); err != nil {
		logx.Error("gateway: %v", err)
		os.Exit(1)
	}
}
