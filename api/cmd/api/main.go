package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"boat/api/internal/config"
	"boat/api/internal/db"
	"boat/api/internal/httpx"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	srv := &http.Server{
		Handler:           httpx.NewRouter(cfg, pool),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// ═══ จุดที่พังบ่อยที่สุดบน Railway ═══
	// private network ของ Railway เป็น IPv6 ถ้า bind "0.0.0.0:PORT" ฝั่ง Astro
	// จะเรียก api.railway.internal ไม่ติด (502) — ต้อง bind ":PORT" เฉยๆ
	// Go จะเปิด [::]:PORT แบบ dual-stack ให้ ใช้ได้ทั้ง public และ private
	addr := ":" + cfg.Port
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen", "addr", addr, "err", err)
		os.Exit(1)
	}
	slog.Info("api listening", "addr", ln.Addr().String(), "env", cfg.Env)

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("serve", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "err", err)
	}
}
