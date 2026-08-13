// Railway Cron Service — รันแล้วต้อง exit(0) ไม่ใช่ process ที่อยู่ยาว
// ตั้ง cronSchedule ไว้ที่ railway.cron.json (ตี 1 ตามเวลาไทย = 18:00 UTC)
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"boat/api/internal/config"
	"boat/api/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

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

	if err := enforceFreeTierLimit(ctx, pool); err != nil {
		slog.Error("enforce free tier", "err", err)
		os.Exit(1)
	}
	if err := reportExpiringSoon(ctx, pool); err != nil {
		slog.Error("expiring soon", "err", err)
		os.Exit(1)
	}
	slog.Info("cron done")
}

// pro หมดอายุ -> เหลือโพสได้ 1 ชิ้น ที่เกินมาให้ซ่อน (เก็บอันที่ bump ล่าสุดไว้)
// ไม่ลบทิ้ง เพราะ URL ต้องอยู่ต่อเพื่อ SEO — หน้าเว็บจะขึ้น noindex แทน
func enforceFreeTierLimit(ctx context.Context, pool *pgxpool.Pool) error {
	const q = `
		WITH ranked AS (
			SELECT p.id,
			       row_number() OVER (PARTITION BY p.user_id ORDER BY p.bumped_at DESC) AS rn
			FROM posts p
			JOIN users u ON u.id = p.user_id
			WHERE p.status = 'active'
			  AND (u.pro_until IS NULL OR u.pro_until <= now())
		)
		UPDATE posts SET status = 'hidden', updated_at = now()
		WHERE id IN (SELECT id FROM ranked WHERE rn > 1)`

	tag, err := pool.Exec(ctx, q)
	if err != nil {
		return err
	}
	slog.Info("free tier enforced", "posts_hidden", tag.RowsAffected())
	return nil
}

// ใครใกล้หมดอายุใน 7/3/1 วัน — phase 1 แค่ log ไว้ก่อน
// phase ถัดไปต่อ LINE Messaging API ยิงเตือน (ฟรี และคนไทยเปิดอ่านแน่นอนกว่าอีเมล)
func reportExpiringSoon(ctx context.Context, pool *pgxpool.Pool) error {
	const q = `
		SELECT id, display_name, line_user_id, email, pro_until
		FROM users
		WHERE pro_until IS NOT NULL
		  AND pro_until > now()
		  AND pro_until <= now() + interval '7 days'
		ORDER BY pro_until`

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var id int64
		var name string
		var lineID, email *string
		var proUntil time.Time
		if err := rows.Scan(&id, &name, &lineID, &email, &proUntil); err != nil {
			return err
		}
		slog.Info("renewal reminder due",
			"user_id", id, "name", name,
			"days_left", int(time.Until(proUntil).Hours()/24))
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	slog.Info("expiring soon", "count", n)
	return nil
}
