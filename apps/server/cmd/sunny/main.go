package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/sunny/sunny/apps/server/internal/alerts"
	"github.com/sunny/sunny/apps/server/internal/auth"
	"github.com/sunny/sunny/apps/server/internal/bus"
	"github.com/sunny/sunny/apps/server/internal/config"
	"github.com/sunny/sunny/apps/server/internal/connectors"

	// Side-effect import: every connector compiled into the binary is
	// registered via this package's init().
	_ "github.com/sunny/sunny/apps/server/internal/connectors/builtins"

	"github.com/sunny/sunny/apps/server/internal/httpapi"
	"github.com/sunny/sunny/apps/server/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfgPath := os.Getenv("SUNNY_CONFIG")
	if cfgPath == "" {
		cfgPath = "sunny.config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logger.Error("load config", "err", err)
		os.Exit(1)
	}
	if v := os.Getenv("SUNNY_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("SUNNY_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}

	specs, err := cfg.ToInstanceSpecs()
	if err != nil {
		logger.Error("config invalid", "err", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		logger.Error("create data dir", "dir", cfg.DataDir, "err", err)
		os.Exit(1)
	}
	dbPath := filepath.Join(cfg.DataDir, "sunny.duckdb")
	store, err := storage.Open(dbPath)
	if err != nil {
		logger.Error("open storage", "path", dbPath, "err", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()
	logger.Info("storage opened", "path", dbPath)

	b := bus.New(256, 64)

	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()

	// Writer must subscribe to the bus before any connector starts publishing.
	writer, writerDone := storage.NewWriter(rootCtx, b, store, logger, storage.DefaultWriterConfig())
	_ = writer

	// Alert engine subscribes before connectors start so we don't miss early records.
	alertEngine := alerts.New(b, store, logger)
	if err := alertEngine.SeedDefaultRule(rootCtx); err != nil {
		logger.Warn("seed default alert rule", "err", err)
	}
	alertDone := make(chan struct{})
	go func() {
		defer close(alertDone)
		if err := alertEngine.Run(rootCtx); err != nil && err != context.Canceled {
			logger.Warn("alert engine exit", "err", err)
		}
	}()

	rt := connectors.NewRuntime(b, logger, connectors.EnvSecrets{}, store)
	for _, spec := range specs {
		if err := rt.Start(rootCtx, spec); err != nil {
			logger.Error("start connector", "instance", spec.InstanceID, "err", err)
			os.Exit(1)
		}
	}

	authMgr, err := auth.NewManager(
		os.Getenv("SUNNY_PASSWORD_HASH"),
		os.Getenv("SUNNY_SESSION_KEY"),
		os.Getenv("SUNNY_API_TOKENS"),
	)
	if err != nil {
		logger.Error("auth manager", "err", err)
		os.Exit(1)
	}
	switch {
	case authMgr.PasswordEnabled() && authMgr.Enabled():
		logger.Info("auth: password + token auth ENABLED")
	case authMgr.PasswordEnabled():
		logger.Info("auth: password auth ENABLED")
	case authMgr.Enabled():
		logger.Info("auth: token-only auth ENABLED")
	default:
		logger.Info("auth: DISABLED (set SUNNY_PASSWORD_HASH or SUNNY_API_TOKENS to enable)")
	}

	queryRPM := 10
	if v := os.Getenv("SUNNY_QUERY_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			queryRPM = n
		}
	}
	if v := os.Getenv("SUNNY_MAX_STREAM_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			httpapi.MaxStreamConnections = int32(n)
		}
	}

	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Logger: logger, Runtime: rt, Bus: b, Storage: store,
			Auth: authMgr, QueryRPM: queryRPM, DataDir: cfg.DataDir,
			CORSOrigins: os.Getenv("SUNNY_CORS_ORIGINS"),
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("sunny server listening", "addr", cfg.Addr, "instances", len(specs))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown", "err", err)
	}
	rt.StopAll(5 * time.Second)
	cancelRoot()
	// Wait for the writer + alert engine to exit before closing storage.
	select {
	case <-writerDone:
	case <-time.After(5 * time.Second):
		logger.Warn("writer didn't finish in time; some records may be lost")
	}
	select {
	case <-alertDone:
	case <-time.After(2 * time.Second):
	}
}
