// Command mosaid-cli runs the Mosaid owner agent in a terminal.
//
// It uses the same config file, model client, tool registry, policy engine,
// approval engine, memory store and session store as the Telegram daemon, so a
// coding session on a laptop exercises exactly the code that runs on the phone.
// It needs a model API key and nothing else: no bot token, no inbound ports.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/agent"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/config"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/console"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/memory"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

var version = "dev"

func main() {
	cfgPath := flag.String("config", "", "config file (same format as the daemon)")
	workspace := flag.String("workspace", "", "override workspace directory")
	oneShot := flag.String("message", "", "run a single message and exit")
	dataDir := flag.String("data-dir", "", "override data directory (isolated state)")
	flag.Parse()
	if *cfgPath == "" {
		fail("--config required")
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fail("config: " + err.Error())
	}
	if *workspace != "" {
		absolute, absErr := filepath.Abs(*workspace)
		if absErr != nil {
			fail("workspace: " + absErr.Error())
		}
		cfg.WorkspaceDir = filepath.Clean(absolute)
	}
	if *dataDir != "" {
		cfg.DataDir = filepath.Clean(*dataDir)
	}
	if err = os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		fail("data dir: " + err.Error())
	}

	key, err := config.ReadSecret(cfg.Model.APIKeyFile)
	if err != nil {
		fail("model secret: " + err.Error())
	}

	lock, err := security.Acquire(cfg.DataDir)
	if err != nil {
		fail("another Mosaid instance owns this data dir; stop it or pass --data-dir")
	}
	defer lock.Release()

	modelClient := model.NewOpenAI(cfg.Model.BaseURL, key, cfg.Model.Name,
		time.Duration(cfg.Model.TimeoutSeconds)*time.Second, int64(cfg.Limits.MaxResponseBytes)).
		Configure(attempts(cfg.Limits.MaxRetries), nil)
	db, err := storage.Open(filepath.Join(cfg.DataDir, "mosaid.db"))
	if err != nil {
		fail("database: " + err.Error())
	}
	defer db.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = db.IntegrityCheck(ctx); err != nil {
		fail("database integrity: " + err.Error())
	}
	sessions := storage.NewSessionStore(filepath.Join(cfg.DataDir, "state"))
	approvals := &approval.Manager{DB: db.SQL(), Audit: audit.Logger{DB: db.SQL()}}
	memoryStore := &memory.Store{DB: db.SQL()}

	registry := tools.NewRegistry(approvals)
	if err = tools.RegisterMemory(registry, memoryStore); err != nil {
		fail("memory tools: " + err.Error())
	}
	if cfg.WorkspaceDir != "" {
		if err = tools.RegisterWorkspace(registry, tools.NewWorkspace(cfg.WorkspaceDir), nil); err != nil {
			fail("workspace tools: " + err.Error())
		}
	}
	catalog, err := registry.Catalog()
	if err != nil {
		fail("tool catalog: " + err.Error())
	}

	owner := &agent.Agent{
		Model: modelClient, Sessions: sessions, Health: health.New(cfg.DataDir, version),
		Version: version, Approvals: approvals, Memory: memoryStore,
		Limits: security.BudgetLimits{
			ModelSteps: cfg.Limits.MaxModelSteps, ToolCalls: cfg.Limits.MaxToolCalls,
			Tokens: cfg.Limits.MaxTokens, CostUSD: cfg.Limits.MaxCostUSD, Retries: cfg.Limits.MaxRetries,
		},
		Prices: security.PriceLimits{InputPer1M: cfg.Model.InputPricePer1M, OutputPer1M: cfg.Model.OutputPricePer1M},
		Tools:  registry, WorkspaceRoot: cfg.WorkspaceDir, SystemPrompt: cfg.Agent.SystemPrompt,
		MaxSteps: cfg.Agent.MaxSteps, MaxCallsPerStep: cfg.Agent.MaxCallsPerStep,
		MaxToolResultBytes: cfg.Agent.MaxToolResultBytes, MaxTokens: cfg.Agent.MaxTokens,
		Mode: policy.Admin,
	}

	if *oneShot != "" {
		out, handleErr := owner.Handle(ctx, message.Inbound{ChatID: 1, MessageID: 1, UserID: cfg.OwnerTelegramID, ChatType: "private", Text: *oneShot})
		if handleErr != nil {
			fail(handleErr.Error())
		}
		fmt.Println(out.Text)
		return
	}

	banner := fmt.Sprintf("Mosaid %s — model %s · %d tools · workspace %s\nType /help, or /exit to quit. Writes need /approve <token>.",
		version, cfg.Model.Name, len(catalog), displayPath(cfg.WorkspaceDir))
	shell := &console.Console{Handler: owner, Out: os.Stdout, UserID: cfg.OwnerTelegramID, ChatID: 1, Banner: banner}
	if err = shell.Run(ctx, os.Stdin); err != nil {
		fail(err.Error())
	}
}

func displayPath(path string) string {
	if path == "" {
		return "(none: file tools disabled, set workspace_dir)"
	}
	return path
}

func attempts(maxRetries int) int {
	if maxRetries < 1 {
		return 1
	}
	if maxRetries > 4 {
		return 4
	}
	return maxRetries
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "mosaid-cli:", message)
	os.Exit(1)
}
