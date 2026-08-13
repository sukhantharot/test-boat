package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"boat/api/internal/config"
)

type Server struct {
	cfg  config.Config
	pool *pgxpool.Pool
}

func NewRouter(cfg config.Config, pool *pgxpool.Pool) http.Handler {
	s := &Server{cfg: cfg, pool: pool}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(s.cors)

	// Railway ยิงเช็คที่นี่ (healthcheckPath ใน railway.json)
	r.Get("/healthz", s.health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/categories", s.listCategories)
		r.Get("/posts", s.listPosts)
		r.Get("/posts/{slug}", s.getPost)

		// TODO phase 1:
		//   POST /auth/line/callback, /auth/google/callback
		//   GET|POST|PATCH /me/posts        (free = 1 โพส, pro = 10)
		//   POST /posts/{id}/leads          (นับ lead + เปิดเบอร์ถ้าเจ้าของเป็น pro)
		//   POST /billing/promptpay         (สร้าง QR)
		//   POST /webhooks/omise            (ตรวจ signature -> insert payments -> ต่อ pro_until)
		//   /admin/*                        (JWT คนละ secret กับฝั่ง user)
	})

	return r
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.cfg.WebOrigin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Add("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "db": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "env": s.cfg.Env})
}

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(),
		`SELECT id, slug, name_th FROM categories ORDER BY sort_order`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id int
		var slug, name string
		if err := rows.Scan(&id, &slug, &name); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		out = append(out, map[string]any{"id": id, "slug": slug, "name": name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// ฟีดหลัก — จุดสำคัญคือ ORDER BY: pro ขึ้นก่อนเสมอ นี่คือสิ่งที่ 300฿ ซื้อ
//
// ⚠ ต้อง coalesce(..., false) เสมอ: free tier มี pro_until = NULL และ
// `NULL > now()` ให้ผลเป็น NULL ไม่ใช่ false — ซึ่ง Postgres จัด NULLS FIRST ตอน DESC
// ถ้าลืมจะกลายเป็น "คนไม่จ่ายขึ้นบนสุด" กลับด้านกับโมเดลธุรกิจทั้งหมด
func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	const q = `
		SELECT p.id, p.slug, p.title, p.province, p.price_from_satang,
		       u.display_name, coalesce(u.pro_until > now(), false) AS is_pro
		FROM posts p
		JOIN users u ON u.id = p.user_id
		WHERE p.status = 'active' AND u.status = 'active'
		ORDER BY coalesce(u.pro_until > now(), false) DESC, p.bumped_at DESC
		LIMIT 24`

	rows, err := s.pool.Query(r.Context(), q)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var slug, title string
		var province *string
		var price *int64
		var author string
		var isPro bool
		if err := rows.Scan(&id, &slug, &title, &province, &price, &author, &isPro); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		out = append(out, map[string]any{
			"id": id, "slug": slug, "title": title, "province": province,
			"price_from_satang": price, "author": author, "is_pro": isPro,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// คืนโพสแม้ status ไม่ใช่ 'active' โดยตั้งใจ — หน้าเว็บต้องไม่ 404
// ให้ Astro เอา field `visible` ไปตัดสินใจว่าจะใส่ noindex ไหม (ดู web/src/pages/services/[slug].astro)
func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	const q = `
		SELECT p.id, p.slug, p.title, p.description, p.province,
		       p.price_from_satang, p.price_unit, p.images, p.status,
		       u.display_name, coalesce(u.pro_until > now(), false) AS is_pro,
		       CASE WHEN u.pro_until > now() THEN p.contact_phone END,
		       CASE WHEN u.pro_until > now() THEN p.contact_line  END
		FROM posts p
		JOIN users u ON u.id = p.user_id
		WHERE p.slug = $1 AND p.status <> 'banned' AND u.status = 'active'`

	var (
		id                         int64
		title, description, status string
		outSlug, author            string
		province, priceUnit        *string
		price                      *int64
		images                     []byte
		isPro                      bool
		contactPhone, contactLine  *string
	)
	err := s.pool.QueryRow(r.Context(), q, slug).Scan(
		&id, &outSlug, &title, &description, &province,
		&price, &priceUnit, &images, &status,
		&author, &isPro, &contactPhone, &contactLine)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id": id, "slug": outSlug, "title": title, "description": description,
		"province": province, "price_from_satang": price, "price_unit": priceUnit,
		"images": json.RawMessage(images), "author": author, "is_pro": isPro,
		"visible":       status == "active",
		"contact_phone": contactPhone, // NULL ถ้าเจ้าของเป็น free tier
		"contact_line":  contactLine,
	}})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
