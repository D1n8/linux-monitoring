package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"goinmetrics/internal/collector"
	"goinmetrics/internal/monitor"
	"goinmetrics/internal/server"
	"goinmetrics/internal/store"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func run() error {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		r, e := client.Get("http://" + env("LISTEN_ADDR", "127.0.0.1:9099") + "/api/health")
		if e != nil {
			return e
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("health status: %d", r.StatusCode)
		}
		return nil
	}
	addr := env("LISTEN_ADDR", "127.0.0.1:9099")
	host, _, err := net.SplitHostPort(addr)
	if err != nil || (host != "127.0.0.1" && host != "::1") {
		return fmt.Errorf("LISTEN_ADDR должен указывать loopback-адрес")
	}
	// Bind before migrating so a second instance fails without changing shared state.
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("порт занят или недоступен: %w", err)
	}
	defer listener.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	initCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	db, err := store.Open(initCtx, env("DATABASE_URL", ""))
	if err != nil {
		return err
	}
	defer db.Pool.Close()
	c := &collector.Collector{Proc: env("HOST_PROC", "/proc"), Root: env("HOST_ROOT", "/")}
	enabled, err := db.Enabled(initCtx)
	if err != nil {
		return fmt.Errorf("состояние мониторинга: %w", err)
	}
	m := monitor.New(c, db, enabled)
	done := make(chan struct{})
	go func() { defer close(done); m.Run(ctx) }()
	httpServer := &http.Server{Handler: server.New(m), BaseContext: func(net.Listener) context.Context { return ctx }, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(listener) }()
	slog.Info("GoInMetrics готов", "url", "http://"+addr, "ssh", "ssh -N -L 127.0.0.1:19099:127.0.0.1:9099 user@server")
	select {
	case <-ctx.Done():
	case err = <-errCh:
		if err != http.ErrServerClosed {
			stop()
		}
	}
	stop()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancelShutdown()
	_ = httpServer.Shutdown(shutdown)
	<-done
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		slog.Error("GoInMetrics", "error", err)
		os.Exit(1)
	}
}
