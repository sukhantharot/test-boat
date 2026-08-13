package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"boat/api/internal/auth"
	"boat/api/internal/demo"
)

// หลังบ้าน — แยกจากฝั่ง user โดยสิ้นเชิง: คนละตาราง คนละ secret คนละ cookie
// ทุกการแก้ข้อมูลบันทึกลง audit_logs เสมอ

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	var (
		id   int64
		hash string
	)
	err := s.pool.QueryRow(r.Context(),
		`SELECT id, password_hash FROM admins WHERE email = $1`,
		strings.ToLower(strings.TrimSpace(req.Email))).Scan(&id, &hash)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !auth.CheckPassword(hash, req.Password)) {
		writeErr(w, http.StatusUnauthorized, "อีเมลหรือรหัสผ่านไม่ถูกต้อง")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "เข้าสู่ระบบไม่สำเร็จ")
		return
	}

	token, err := auth.Sign(s.cfg.JWTAdminSecret, auth.AudienceAdmin, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "สร้าง session ไม่สำเร็จ")
		return
	}
	auth.SetCookie(w, auth.CookieAdmin, token, s.cfg.CookieSecure())

	_, _ = s.pool.Exec(r.Context(), `UPDATE admins SET last_login_at = now() WHERE id = $1`, id)
	s.audit(r.Context(), id, "admin.login", "admin", id, nil)

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id}})
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearCookie(w, auth.CookieAdmin, s.cfg.CookieSecure())
	writeJSON(w, http.StatusOK, map[string]any{"data": "ออกจากระบบแล้ว"})
}

func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) {
	var email, name, role string
	if err := s.pool.QueryRow(r.Context(),
		`SELECT email::text, name, role FROM admins WHERE id = $1`,
		adminID(r)).Scan(&email, &name, &role); err != nil {
		writeErr(w, http.StatusUnauthorized, "ไม่พบบัญชีผู้ดูแล")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id": adminID(r), "email": email, "name": name, "role": role,
	}})
}

// ตัวเลขหน้าแรกของหลังบ้าน — คำถามที่ admin ถามทุกวัน รวมไว้ query เดียว
func (s *Server) adminStats(w http.ResponseWriter, r *http.Request) {
	var (
		users, proUsers, newUsers7d       int
		posts, activePosts, newPosts7d    int
		leads, leads7d                    int
		revenueSatang, revenueSatang30d   int64
	)
	err := s.pool.QueryRow(r.Context(), `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM users WHERE pro_until > now()),
			(SELECT count(*) FROM users WHERE created_at > now() - interval '7 days'),
			(SELECT count(*) FROM posts WHERE status <> 'banned'),
			(SELECT count(*) FROM posts WHERE status = 'active'),
			(SELECT count(*) FROM posts WHERE created_at > now() - interval '7 days'),
			(SELECT count(*) FROM leads),
			(SELECT count(*) FROM leads WHERE created_at > now() - interval '7 days'),
			(SELECT coalesce(sum(amount_satang), 0) FROM payments WHERE status = 'paid'),
			(SELECT coalesce(sum(amount_satang), 0) FROM payments
			 WHERE status = 'paid' AND paid_at > now() - interval '30 days')
	`).Scan(&users, &proUsers, &newUsers7d, &posts, &activePosts, &newPosts7d,
		&leads, &leads7d, &revenueSatang, &revenueSatang30d)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"users": users, "pro_users": proUsers, "new_users_7d": newUsers7d,
		"posts": posts, "active_posts": activePosts, "new_posts_7d": newPosts7d,
		"leads": leads, "leads_7d": leads7d,
		"revenue_satang": revenueSatang, "revenue_satang_30d": revenueSatang30d,
		// อัตราแปลงเป็น Pro — ถ้าตัวเลขนี้ต่ำมาก แปลว่าคุณค่าที่ให้ยังไม่พอ
		"pro_conversion_pct": pct(proUsers, users),
	}})
}

func pct(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) * 100 / float64(total)
}

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50, 1, 200)
	offset := queryInt(r, "offset", 0, 0, 1000000)
	q := trim(r.URL.Query().Get("q"), 80)

	rows, err := s.pool.Query(r.Context(), `
		SELECT u.id, coalesce(u.email::text, ''), u.display_name, u.phone, u.provider,
		       u.status, u.pro_until, coalesce(u.pro_until > now(), false), u.created_at,
		       (SELECT count(*) FROM posts p WHERE p.user_id = u.id AND p.status = 'active'),
		       (SELECT count(*) FROM leads l JOIN posts p ON p.id = l.post_id WHERE p.user_id = u.id),
		       count(*) OVER () AS total
		FROM users u
		WHERE ($1 = '' OR u.display_name ILIKE '%'||$1||'%' OR u.email::text ILIKE '%'||$1||'%')
		ORDER BY u.created_at DESC
		LIMIT $2 OFFSET $3`, q, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	total := 0
	for rows.Next() {
		var (
			id                            int64
			email, name, provider, status string
			phone                         *string
			proUntil                      *time.Time
			isPro                         bool
			createdAt                     time.Time
			activePosts, leads            int
		)
		if err := rows.Scan(&id, &email, &name, &phone, &provider, &status, &proUntil,
			&isPro, &createdAt, &activePosts, &leads, &total); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]any{
			"id": id, "email": email, "display_name": name, "phone": phone,
			"provider": provider, "status": status, "pro_until": proUntil, "is_pro": isPro,
			"created_at": createdAt, "active_posts": activePosts, "total_leads": leads,
		})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// ดู comment เรื่อง count(*) OVER () ใน listPosts (posts_handlers.go)
	// offset เลยข้อมูล -> ไม่มีแถวให้นับ -> total ค้าง 0 -> pager หลังบ้านหาย
	if len(out) == 0 && offset > 0 {
		if err := s.pool.QueryRow(r.Context(), `
			SELECT count(*) FROM users u
			WHERE ($1 = '' OR u.display_name ILIKE '%'||$1||'%' OR u.email::text ILIKE '%'||$1||'%')`,
			q).Scan(&total); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": out, "meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}

// แก้ผู้ใช้: ระงับ/คืนสิทธิ์ และ "แถมวัน Pro ฟรี" ซึ่งต้องมีแน่นอนตอนช่วงเปิดตัว
func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "id ไม่ถูกต้อง")
		return
	}
	var req struct {
		Status   *string `json:"status"`    // active | suspended
		GrantPro *int    `json:"grant_pro"` // จำนวนวันที่แถมให้ (ติดลบ = ตัดออก)
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "suspended" {
			writeErr(w, http.StatusBadRequest, "status ต้องเป็น active หรือ suspended")
			return
		}
		if _, err := s.pool.Exec(r.Context(),
			`UPDATE users SET status = $1, updated_at = now() WHERE id = $2`,
			*req.Status, id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.audit(r.Context(), adminID(r), "user.status", "user", id,
			map[string]any{"status": *req.Status})
	}

	if req.GrantPro != nil && *req.GrantPro != 0 {
		days := *req.GrantPro
		if days > 365 || days < -365 {
			writeErr(w, http.StatusBadRequest, "แถมได้ครั้งละไม่เกิน 365 วัน")
			return
		}
		// ต่อจากวันหมดอายุเดิมถ้ายังไม่หมด ไม่งั้นเริ่มนับจากวันนี้
		if _, err := s.pool.Exec(r.Context(), `
			UPDATE users
			SET pro_until = greatest(coalesce(pro_until, now()), now()) + make_interval(days => $1),
			    updated_at = now()
			WHERE id = $2`, days, id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		// ลงบัญชีเป็น payment method='comp' ด้วย ไม่งั้นรายงานรายได้จะเพี้ยน
		// (เห็น Pro เพิ่มแต่หาเงินไม่เจอ) และไล่ย้อนไม่ได้ว่าใครแถมให้
		if days > 0 {
			if _, err := s.pool.Exec(r.Context(), `
				INSERT INTO payments (user_id, amount_satang, method, provider, status,
				                      days_granted, paid_at)
				VALUES ($1, 1, 'comp', 'manual', 'paid', $2, now())`, id, days); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		s.audit(r.Context(), adminID(r), "user.grant_pro", "user", id,
			map[string]any{"days": days})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": "บันทึกแล้ว"})
}

func (s *Server) adminListPosts(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50, 1, 200)
	offset := queryInt(r, "offset", 0, 0, 1000000)
	q := trim(r.URL.Query().Get("q"), 80)
	status := trim(r.URL.Query().Get("status"), 20)

	rows, err := s.pool.Query(r.Context(), `
		SELECT p.id, p.slug, p.title, p.status, p.view_count, p.contact_count,
		       p.created_at, u.id, u.display_name,
		       coalesce(u.pro_until > now(), false),
		       count(*) OVER () AS total
		FROM posts p JOIN users u ON u.id = p.user_id
		WHERE ($1 = '' OR p.title ILIKE '%'||$1||'%' OR u.display_name ILIKE '%'||$1||'%')
		  AND ($2 = '' OR p.status = $2)
		ORDER BY p.created_at DESC
		LIMIT $3 OFFSET $4`, q, status, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	total := 0
	for rows.Next() {
		var (
			id, uid              int64
			slug, title, status  string
			views, contacts      int
			createdAt            time.Time
			author               string
			isPro                bool
		)
		if err := rows.Scan(&id, &slug, &title, &status, &views, &contacts,
			&createdAt, &uid, &author, &isPro, &total); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]any{
			"id": id, "slug": slug, "title": title, "status": status,
			"view_count": views, "contact_count": contacts, "created_at": createdAt,
			"user_id": uid, "author": author, "is_pro": isPro,
		})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(out) == 0 && offset > 0 {
		if err := s.pool.QueryRow(r.Context(), `
			SELECT count(*) FROM posts p JOIN users u ON u.id = p.user_id
			WHERE ($1 = '' OR p.title ILIKE '%'||$1||'%' OR u.display_name ILIKE '%'||$1||'%')
			  AND ($2 = '' OR p.status = $2)`,
			q, status).Scan(&total); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": out, "meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}

func (s *Server) adminUpdatePost(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "id ไม่ถูกต้อง")
		return
	}
	var req struct {
		Status string `json:"status"` // active | hidden | banned
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	switch req.Status {
	case "active", "hidden", "banned":
	default:
		writeErr(w, http.StatusBadRequest, "status ไม่ถูกต้อง")
		return
	}

	tag, err := s.pool.Exec(r.Context(),
		`UPDATE posts SET status = $1, updated_at = now() WHERE id = $2`, req.Status, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "ไม่พบประกาศนี้")
		return
	}
	s.audit(r.Context(), adminID(r), "post.status", "post", id,
		map[string]any{"status": req.Status})
	writeJSON(w, http.StatusOK, map[string]any{"data": "บันทึกแล้ว"})
}

func (s *Server) adminListPayments(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50, 1, 200)
	offset := queryInt(r, "offset", 0, 0, 1000000)

	rows, err := s.pool.Query(r.Context(), `
		SELECT p.id, p.amount_satang, p.method, p.provider, p.status, p.days_granted,
		       p.paid_at, p.created_at, p.slip_url, u.id, u.display_name,
		       count(*) OVER () AS total
		FROM payments p JOIN users u ON u.id = p.user_id
		ORDER BY p.created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	total := 0
	for rows.Next() {
		var (
			id, uid           int64
			amount            int64
			method, status    string
			provider, slipURL *string
			days              int
			paidAt            *time.Time
			createdAt         time.Time
			author            string
		)
		if err := rows.Scan(&id, &amount, &method, &provider, &status, &days,
			&paidAt, &createdAt, &slipURL, &uid, &author, &total); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]any{
			"id": id, "amount_satang": amount, "method": method, "provider": provider,
			"status": status, "days_granted": days, "paid_at": paidAt,
			"created_at": createdAt, "slip_url": slipURL,
			"user_id": uid, "author": author,
		})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(out) == 0 && offset > 0 {
		// ต้อง JOIN users เหมือน query ข้างบนเป๊ะๆ ไม่งั้นตัวเลขสองที่จะไม่ตรงกัน
		if err := s.pool.QueryRow(r.Context(),
			`SELECT count(*) FROM payments p JOIN users u ON u.id = p.user_id`).Scan(&total); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": out, "meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}

// ── ข้อมูลตัวอย่างสำหรับสาธิต ────────────────────────────────────────────
//
// ทำเป็น endpoint เพราะบน Railway ต่อ Postgres จากเครื่องตัวเองไม่ได้
// (ไม่ได้เปิด public access) การรันผ่าน api จึงเป็นทางเดียวที่ไม่ต้องเปิด DB ออกเน็ต

func (s *Server) adminSeedDemo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		N int `json:"n"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.N <= 0 {
		req.N = 1000
	}

	// สร้างหลักพันแถวใช้เวลาหลายวินาที ต้องมี ctx ของตัวเองไม่ให้ถูกตัดกลางคัน
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Minute)
	defer cancel()

	res, err := demo.Seed(ctx, s.pool, req.N, time.Now().UnixNano())
	if err != nil {
		slog.Error("seed demo", "err", err)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(ctx, adminID(r), "demo.seed", "system", 0,
		map[string]any{"posts": res.Posts, "users": res.Users})

	writeJSON(w, http.StatusCreated, map[string]any{"data": res})
}

func (s *Server) adminPurgeDemo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Minute)
	defer cancel()

	deleted, err := demo.Purge(ctx, s.pool)
	if err != nil {
		slog.Error("purge demo", "err", err)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(ctx, adminID(r), "demo.purge", "system", 0,
		map[string]any{"users_deleted": deleted})

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"users_deleted": deleted},
	})
}

// audit ต้องไม่ทำให้ request หลักพัง ถ้าเขียน log ไม่ได้ก็ปล่อยผ่าน
func (s *Server) audit(ctx context.Context, adminID int64, action, targetType string, targetID int64, meta map[string]any) {
	raw := []byte(`{}`)
	if meta != nil {
		if b, err := json.Marshal(meta); err == nil {
			raw = b
		}
	}
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO audit_logs (admin_id, action, target_type, target_id, meta)
		VALUES ($1, $2, $3, $4, $5)`, adminID, action, targetType, targetID, raw)
}
