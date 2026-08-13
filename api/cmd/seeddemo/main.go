// CLI สร้างประกาศตัวอย่างสำหรับสาธิตลูกค้า
//
//	go run ./cmd/seeddemo -n 1000     สร้าง 1,000 ประกาศ
//	go run ./cmd/seeddemo -purge      ลบข้อมูลตัวอย่างทั้งหมดทิ้ง
//
// บน Railway ใช้ผ่านหน้าหลังบ้านแทน (Postgres ไม่ได้เปิด public access
// จึงต่อจากเครื่องตัวเองไม่ได้) — ตรรกะเดียวกันอยู่ที่ internal/demo
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"boat/api/internal/config"
	"boat/api/internal/db"
	"boat/api/internal/demo"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	n := flag.Int("n", 1000, "จำนวนประกาศตัวอย่างที่จะสร้าง")
	purge := flag.Bool("purge", false, "ลบข้อมูลตัวอย่างทั้งหมดทิ้ง")
	seed := flag.Int64("seed", 20260813, "seed ของ random ให้ผลลัพธ์ซ้ำได้")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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

	if *purge {
		deleted, err := demo.Purge(ctx, pool)
		if err != nil {
			slog.Error("purge", "err", err)
			os.Exit(1)
		}
		slog.Info("ลบข้อมูลตัวอย่างแล้ว", "users_deleted", deleted)
		return
	}

	res, err := demo.Seed(ctx, pool, *n, *seed)
	if err != nil {
		slog.Error("seed", "err", err)
		os.Exit(1)
	}
	slog.Info("สร้างข้อมูลตัวอย่างแล้ว",
		"posts", res.Posts, "users", res.Users, "pro_users", res.ProUsers,
		"ลบทิ้งด้วย", "seeddemo -purge")
}
