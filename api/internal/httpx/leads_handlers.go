package httpx

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// ═══ นี่คือตัวเลขที่ใช้ปิดการขาย 300฿ ═══
// ตอนหมดช่วงฟรี คนจะยอมจ่ายก็ต่อเมื่อเห็นว่า "เดือนที่แล้วมีคนติดต่อ 18 ราย"
// ถ้าไม่มีตัวเลขนี้ ไม่มีทางขายค่าต่ออายุได้เลย

type leadReq struct {
	Channel string `json:"channel"` // phone | line | form
	Message string `json:"message"`
}

func (s *Server) createLead(w http.ResponseWriter, r *http.Request) {
	var req leadReq
	if !decodeJSON(w, r, &req) {
		return
	}
	switch req.Channel {
	case "phone", "line", "form":
	default:
		writeErr(w, http.StatusBadRequest, "ช่องทางติดต่อไม่ถูกต้อง")
		return
	}

	var (
		postID, ownerID int64
		isPro           bool
		phone, line     *string
	)
	err := s.pool.QueryRow(r.Context(), `
		SELECT p.id, p.user_id, coalesce(u.pro_until > now(), false),
		       p.contact_phone, p.contact_line
		FROM posts p JOIN users u ON u.id = p.user_id
		WHERE p.slug = $1 AND p.status = 'active' AND u.status = 'active'`,
		chi.URLParam(r, "slug")).Scan(&postID, &ownerID, &isPro, &phone, &line)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "ไม่พบประกาศนี้")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	viewer := userID(r)
	if viewer == ownerID {
		writeErr(w, http.StatusBadRequest, "ติดต่อประกาศของตัวเองไม่ได้")
		return
	}

	// คนเดิมกดซ้ำในโพสเดิมภายใน 24 ชม. ให้บันทึกไว้แต่ไม่นับซ้ำ
	// ไม่งั้นตัวเลข lead เฟ้อจนเจ้าของประกาศไม่เชื่อถือ แล้วเราก็ขายต่ออายุไม่ได้
	var dup bool
	if err := s.pool.QueryRow(r.Context(), `
		SELECT exists(
			SELECT 1 FROM leads
			WHERE post_id = $1 AND viewer_user_id = $2 AND created_at > now() - interval '24 hours'
		)`, postID, viewer).Scan(&dup); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO leads (post_id, viewer_user_id, channel, message)
		VALUES ($1, $2, $3, nullif($4, ''))`,
		postID, viewer, req.Channel, trim(req.Message, 1000)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !dup {
		if _, err := tx.Exec(r.Context(),
			`UPDATE posts SET contact_count = contact_count + 1 WHERE id = $1`, postID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// เจ้าของเป็น Pro -> คืนเบอร์ให้เลย  free -> ไม่คืน ผู้จ้างต้องรอเจ้าของติดต่อกลับ
	resp := map[string]any{"recorded": true, "counted": !dup}
	if isPro {
		resp["contact_phone"] = phone
		resp["contact_line"] = line
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": resp})
}

// รายชื่อคนที่สนใจงานเรา — สิทธิ์ Pro เท่านั้น (ตรงกับตารางแพ็กเกจ)
func (s *Server) myLeads(w http.ResponseWriter, r *http.Request) {
	var isPro bool
	if err := s.pool.QueryRow(r.Context(),
		`SELECT coalesce(pro_until > now(), false) FROM users WHERE id = $1`,
		userID(r)).Scan(&isPro); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !isPro {
		writeErr(w, http.StatusPaymentRequired,
			"ดูรายชื่อผู้ติดต่อได้เฉพาะสมาชิก Pro — อัปเกรด 300฿/เดือน")
		return
	}

	rows, err := s.pool.Query(r.Context(), `
		SELECT l.id, l.channel, l.message, l.created_at,
		       p.title, p.slug,
		       coalesce(v.display_name, 'ผู้ใช้ที่ถูกลบ'), v.email::text, v.phone
		FROM leads l
		JOIN posts p ON p.id = l.post_id
		LEFT JOIN users v ON v.id = l.viewer_user_id
		WHERE p.user_id = $1
		ORDER BY l.created_at DESC
		LIMIT 200`, userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var (
			id                   int64
			channel, title, slug string
			viewerName           string
			message              *string
			email, phone         *string
			createdAt            time.Time
		)
		if err := rows.Scan(&id, &channel, &message, &createdAt,
			&title, &slug, &viewerName, &email, &phone); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]any{
			"id": id, "channel": channel, "message": message, "created_at": createdAt,
			"post_title": title, "post_slug": slug,
			"viewer_name": viewerName, "viewer_email": email, "viewer_phone": phone,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}
