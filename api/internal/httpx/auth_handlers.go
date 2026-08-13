package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"boat/api/internal/auth"
)

// Phase 1 ไม่ยืนยันอีเมลตามที่ตกลงกันไว้ — สมัครเสร็จใช้งานได้ทันที
// ตาราง users รองรับ provider 'line'/'google' อยู่แล้ว ต่อ OAuth ทีหลังได้โดยไม่ต้องแก้ schema

type registerReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Phone       string `json:"phone"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if !decodeJSON(w, r, &req) {
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if _, err := mail.ParseAddress(email); err != nil {
		writeErr(w, http.StatusBadRequest, "อีเมลไม่ถูกต้อง")
		return
	}
	if len([]rune(req.Password)) < 8 {
		writeErr(w, http.StatusBadRequest, "รหัสผ่านต้องยาวอย่างน้อย 8 ตัวอักษร")
		return
	}
	name := trim(req.DisplayName, 80)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "กรุณากรอกชื่อที่ใช้แสดง")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "สร้างบัญชีไม่สำเร็จ")
		return
	}

	var id int64
	// ส่ง email เป็น 2 พารามิเตอร์แยกกันทั้งที่ค่าเดียวกัน โดยตั้งใจ:
	// provider_uid เป็น text ส่วน email เป็น citext ถ้าใช้ $1 ร่วมกัน Postgres
	// จะ deduce type ไม่ได้แล้วตอบ "inconsistent types deduced for parameter $1"
	// (การ cast ไม่ช่วย เพราะตัว $1 เองยังต้องมีชนิดเดียว)
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO users (provider, provider_uid, email, password_hash, display_name, phone)
		VALUES ('local', $1, $2, $3, $4, nullif($5, ''))
		RETURNING id`,
		email, email, hash, name, trim(req.Phone, 20)).Scan(&id)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		writeErr(w, http.StatusConflict, "อีเมลนี้ถูกใช้สมัครไปแล้ว")
		return
	}
	if err != nil {
		// ไม่บอกรายละเอียดกับ client แต่ต้องมีใน log ไม่งั้น 500 กลายเป็นปริศนา
		slog.Error("register", "err", err)
		writeErr(w, http.StatusInternalServerError, "สร้างบัญชีไม่สำเร็จ")
		return
	}

	s.issueUserSession(w, id)
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"id": id}})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	var (
		id     int64
		hash   *string
		status string
	)
	err := s.pool.QueryRow(r.Context(), `
		SELECT id, password_hash, status FROM users
		WHERE email = $1 AND provider = 'local'`,
		strings.ToLower(strings.TrimSpace(req.Email))).Scan(&id, &hash, &status)

	// ตอบข้อความเดียวกันทั้งกรณีไม่มีอีเมลนี้และรหัสผิด กัน enumerate ว่าใครสมัครไว้บ้าง
	if errors.Is(err, pgx.ErrNoRows) || hash == nil || !auth.CheckPassword(*hash, req.Password) {
		writeErr(w, http.StatusUnauthorized, "อีเมลหรือรหัสผ่านไม่ถูกต้อง")
		return
	}
	if err != nil {
		slog.Error("login", "err", err)
		writeErr(w, http.StatusInternalServerError, "เข้าสู่ระบบไม่สำเร็จ")
		return
	}
	if status != "active" {
		writeErr(w, http.StatusForbidden, "บัญชีนี้ถูกระงับการใช้งาน")
		return
	}

	s.issueUserSession(w, id)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id}})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	auth.ClearCookie(w, auth.CookieUser, s.cfg.CookieSecure())
	writeJSON(w, http.StatusOK, map[string]any{"data": "ออกจากระบบแล้ว"})
}

func (s *Server) issueUserSession(w http.ResponseWriter, id int64) {
	token, err := auth.Sign(s.cfg.JWTUserSecret, auth.AudienceUser, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "สร้าง session ไม่สำเร็จ")
		return
	}
	auth.SetCookie(w, auth.CookieUser, token, s.cfg.CookieSecure())
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	var (
		id                 int64
		email, name        string
		phone              *string
		proUntil           *time.Time
		isPro              bool
		activePosts, leads int
	)
	err := s.pool.QueryRow(r.Context(), `
		SELECT u.id, coalesce(u.email::text, ''), u.display_name, u.phone,
		       u.pro_until, coalesce(u.pro_until > now(), false),
		       (SELECT count(*) FROM posts p WHERE p.user_id = u.id AND p.status = 'active'),
		       (SELECT count(*) FROM leads l JOIN posts p ON p.id = l.post_id WHERE p.user_id = u.id)
		FROM users u WHERE u.id = $1`,
		userID(r)).Scan(&id, &email, &name, &phone, &proUntil, &isPro, &activePosts, &leads)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "ไม่พบบัญชีผู้ใช้")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id": id, "email": email, "display_name": name, "phone": phone,
		"is_pro": isPro, "pro_until": proUntil,
		"active_posts": activePosts, "post_limit": postLimit(isPro),
		"total_leads": leads,
	}})
}
