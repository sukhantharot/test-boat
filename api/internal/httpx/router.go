package httpx

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
		r.Get("/sitemap/posts", s.sitemapPosts)

		// ────────── สาธารณะ (รู้ว่าใคร login อยู่ก็ดี แต่ไม่บังคับ)
		r.Group(func(r chi.Router) {
			r.Use(s.optionalUser)
			r.Get("/posts", s.listPosts)
			r.Get("/posts/{slug}", s.getPost)
		})

		// ────────── สมัคร / เข้าสู่ระบบ  (phase 1 ไม่ verify อีเมล)
		r.Post("/auth/register", s.register)
		r.Post("/auth/login", s.login)
		r.Post("/auth/logout", s.logout)

		// ────────── ต้อง login
		r.Group(func(r chi.Router) {
			r.Use(s.requireUser)
			r.Get("/me", s.me)
			r.Get("/me/posts", s.myPosts)
			r.Post("/me/posts", s.createPost)
			r.Patch("/me/posts/{id}", s.updatePost)
			r.Delete("/me/posts/{id}", s.deletePost)
			r.Post("/me/posts/{id}/bump", s.bumpPost)
			r.Get("/me/leads", s.myLeads)

			// บังคับ login ก่อนกดติดต่อ — เพื่อเก็บ demand-side user
			// และเพื่อให้ตัวเลข lead เชื่อถือได้พอที่จะเอาไปขายค่าต่ออายุ
			r.Post("/posts/{slug}/leads", s.createLead)
		})

		// ────────── หลังบ้าน (คนละ cookie / คนละ secret กับฝั่ง user)
		r.Route("/admin", func(r chi.Router) {
			r.Post("/login", s.adminLogin)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdmin)
				r.Post("/logout", s.adminLogout)
				r.Get("/me", s.adminMe)
				r.Get("/stats", s.adminStats)
				r.Get("/users", s.adminListUsers)
				r.Patch("/users/{id}", s.adminUpdateUser)
				r.Get("/posts", s.adminListPosts)
				r.Patch("/posts/{id}", s.adminUpdatePost)
				r.Get("/payments", s.adminListPayments)
				r.Post("/demo", s.adminSeedDemo)
				r.Delete("/demo", s.adminPurgeDemo)
			})
		})

		// TODO ถัดไป: /billing/promptpay, /webhooks/omise, OAuth ฝั่ง LINE/Google
	})

	return r
}

// ปกติ Astro proxy ทำให้ทุกอย่างเป็น same-origin อยู่แล้ว CORS จึงแทบไม่ถูกใช้
// เก็บไว้เผื่อกรณีเรียก API ตรงๆ ตอน dev
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id int
		var slug, name string
		if err := rows.Scan(&id, &slug, &name); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]any{"id": id, "slug": slug, "name": name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}
