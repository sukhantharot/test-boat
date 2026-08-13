// รันเป็น preDeployCommand บน Railway — ถ้า migration พังจะไม่ปล่อยเวอร์ชันใหม่ขึ้น
//
// ถ้าตั้ง ADMIN_EMAIL + ADMIN_PASSWORD ไว้ จะ seed/อัปเดตบัญชีผู้ดูแลให้ด้วย
// (idempotent — รันซ้ำกี่รอบก็ได้ผลเดิม) วิธีนี้ทำให้ไม่ต้องมีรหัสผ่านอยู่ใน git
// และไม่ต้องหา shell บน Railway เพื่อรันคำสั่ง one-off
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"boat/api/internal/auth"
	"boat/api/internal/config"
	"boat/api/internal/db"
	"boat/api/internal/migrate"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

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

	if err := seedAdmin(ctx, pool, cfg); err != nil {
		slog.Error("seed admin", "err", err)
		os.Exit(1)
	}
}

func seedAdmin(ctx context.Context, pool *pgxpool.Pool, cfg config.Config) error {
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM admins`).Scan(&n); err == nil && n == 0 {
			slog.Warn("ยังไม่มีบัญชีผู้ดูแลในระบบ — ตั้ง ADMIN_EMAIL และ ADMIN_PASSWORD แล้ว deploy ใหม่เพื่อสร้าง")
		}
		return nil
	}
	if len([]rune(cfg.AdminPassword)) < 12 {
		// หลังบ้านเข้าถึงข้อมูลผู้ใช้ทั้งระบบ รหัสสั้นกว่านี้ไม่ควรผ่าน
		slog.Error("ADMIN_PASSWORD ต้องยาวอย่างน้อย 12 ตัวอักษร")
		os.Exit(1)
	}

	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}

	var id int64
	var created bool
	err = pool.QueryRow(ctx, `
		INSERT INTO admins (email, password_hash, name, role)
		VALUES ($1, $2, $3, 'owner')
		ON CONFLICT (email) DO UPDATE SET password_hash = excluded.password_hash
		RETURNING id, (xmax = 0) AS created`,
		cfg.AdminEmail, hash, "ผู้ดูแลระบบ").Scan(&id, &created)
	if err != nil {
		return err
	}

	if created {
		slog.Info("สร้างบัญชีผู้ดูแลแล้ว", "id", id, "email", cfg.AdminEmail)
	} else {
		slog.Info("อัปเดตรหัสผ่านผู้ดูแลแล้ว", "id", id, "email", cfg.AdminEmail)
	}
	return nil
}
