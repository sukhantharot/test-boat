// รันเป็น preDeployCommand บน Railway — ถ้า migration พังจะไม่ปล่อยเวอร์ชันใหม่ขึ้น
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"boat/api/internal/config"
	"boat/api/internal/db"
	"boat/api/internal/migrate"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := migrate.Up(ctx, pool); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}
}
