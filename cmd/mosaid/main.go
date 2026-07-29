package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/agent"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/config"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/observability"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/telegram"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var version = "dev"
var commit = "unknown"
var buildTime = "unknown"

func main() {
	cfgPath := flag.String("config", "", "config file")
	show := flag.Bool("version", false, "show version")
	flag.Parse()
	if *show {
		fmt.Printf("mosaid %s commit=%s built=%s\n", version, commit, buildTime)
		return
	}
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "--config required")
		os.Exit(2)
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	token, err := config.ReadSecret(cfg.Telegram.TokenFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "telegram secret:", err)
		os.Exit(1)
	}
	key, err := config.ReadSecret(cfg.Model.APIKeyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "model secret:", err)
		os.Exit(1)
	}
	red := observability.NewRedactor(token, key)
	log := observability.New(os.Stdout, red)
	lock, err := security.Acquire(cfg.DataDir)
	if err != nil {
		log.Error("singleton", slog.String("error", err.Error()))
		os.Exit(73)
	}
	defer lock.Release()
	h := health.New(cfg.DataDir, version)
	_ = h.Update(func(s *health.State) { s.Status = "running" })
	defer h.Update(func(s *health.State) { s.Status = "stopped" })
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	m := model.NewOpenAI(cfg.Model.BaseURL, key, cfg.Model.Name, time.Duration(cfg.Model.TimeoutSeconds)*time.Second, int64(cfg.Limits.MaxResponseBytes))
	sessions := storage.NewSessionStore(filepath.Join(cfg.DataDir, "state"))
	a := &agent.Agent{Model: m, Sessions: sessions, Health: h, Version: version}
	g := &telegram.Gateway{Client: telegram.New(token), Handler: a, Owner: cfg.OwnerTelegramID, PollTimeout: cfg.Telegram.PollTimeoutSeconds, Log: log, Health: h}
	log.Info("mosaid started", "version", version, "commit", commit)
	if err = g.Run(ctx); err != nil {
		log.Error("gateway stopped", "error", err.Error())
		os.Exit(1)
	}
}
