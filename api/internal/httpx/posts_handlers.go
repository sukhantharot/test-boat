package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	slugpkg "boat/api/internal/slug"
)

// ลิมิตตามแพ็กเกจ — free tier ถาวรแต่โพสได้ชิ้นเดียว
// 300฿ ซื้อ "จำนวนโพส + อันดับ + เปิดเบอร์" ไม่ใช่ค่าผ่านประตู
const (
	freePostLimit  = 1
	proPostLimit   = 10
	freeImageLimit = 1
	proImageLimit  = 10
)

func postLimit(isPro bool) int {
	if isPro {
		return proPostLimit
	}
	return freePostLimit
}

func imageLimit(isPro bool) int {
	if isPro {
		return proImageLimit
	}
	return freeImageLimit
}

// ───────────────────────────────── public

func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 24, 1, 100)
	offset := queryInt(r, "offset", 0, 0, 100000)
	q := trim(r.URL.Query().Get("q"), 80)
	category := trim(r.URL.Query().Get("category"), 40)
	province := trim(r.URL.Query().Get("province"), 60)
	// count(*) OVER () บังคับอ่านทุกแถวที่ match แม้จะคืนแค่ 24
	// วัดจริงที่ 5,000 โพส = 5ms แต่โตเป็นเชิงเส้น -> 500k โพสจะ ~500ms
	// หน้าที่ไม่ต้องโชว์ "พบ N รายการ" ให้ส่ง ?with_total=0 มาเพื่อข้าม
	withTotal := r.URL.Query().Get("with_total") != "0"

	// ── ประกอบ WHERE แบบไดนามิก ─────────────────────────────────────────
	// เดิมเขียนรวบเป็น ($1 = '' OR title ILIKE ...) เพื่อให้ query เดียวจบ
	// แต่ OR แบบนั้นทำให้ planner ใช้ index posts_title_trgm_idx ไม่ได้เลย
	// (วัดจริง: 11.8ms แบบมี guard เทียบกับ 0.53ms แบบไม่มี)
	conds := []string{"p.status = 'active'", "u.status = 'active'"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if q != "" {
		args = append(args, q)
		n := len(args)
		conds = append(conds, fmt.Sprintf(
			"(p.title ILIKE '%%'||$%d||'%%' OR p.description ILIKE '%%'||$%d||'%%')", n, n))
	}
	if category != "" {
		add("c.slug = $%d", category)
	}
	if province != "" {
		add("p.province = $%d", province)
	}

	totalExpr := "0"
	if withTotal {
		totalExpr = "count(*) OVER ()"
	}
	args = append(args, limit, offset)

	sql := fmt.Sprintf(`
		SELECT p.id, p.slug, p.title, p.province, p.price_from_satang, p.price_unit,
		       p.images, p.bumped_at,
		       u.display_name, coalesce(u.pro_until > now(), false) AS is_pro,
		       c.slug, c.name_th,
		       %s AS total
		FROM posts p
		JOIN users u ON u.id = p.user_id
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE %s
		ORDER BY coalesce(u.pro_until > now(), false) DESC, p.bumped_at DESC
		LIMIT $%d OFFSET $%d`,
		totalExpr, strings.Join(conds, " AND "), len(args)-1, len(args))

	rows, err := s.pool.Query(r.Context(), sql, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	total := 0
	for rows.Next() {
		var (
			id                  int64
			slug, title, author string
			province, unit      *string
			price               *int64
			images              []byte
			bumped              time.Time
			isPro               bool
			catSlug, catName    *string
		)
		if err := rows.Scan(&id, &slug, &title, &province, &price, &unit, &images,
			&bumped, &author, &isPro, &catSlug, &catName, &total); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]any{
			"id": id, "slug": slug, "title": title, "province": province,
			"price_from_satang": price, "price_unit": unit,
			"images": json.RawMessage(images), "bumped_at": bumped,
			"author": author, "is_pro": isPro,
			"category_slug": catSlug, "category_name": catName,
		})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": out,
		"meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}

// คืนโพสแม้ status ไม่ใช่ 'active' โดยตั้งใจ — หน้าเว็บต้องไม่ 404
// ให้ Astro เอา field `visible` ไปตัดสินใจว่าจะใส่ noindex ไหม
func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	const sql = `
		SELECT p.id, p.slug, p.title, p.description, p.province,
		       p.price_from_satang, p.price_unit, p.images, p.status,
		       p.view_count, p.contact_count, p.created_at,
		       p.user_id, u.display_name, coalesce(u.pro_until > now(), false) AS is_pro,
		       CASE WHEN u.pro_until > now() THEN p.contact_phone END,
		       CASE WHEN u.pro_until > now() THEN p.contact_line  END,
		       c.slug, c.name_th
		FROM posts p
		JOIN users u ON u.id = p.user_id
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE p.slug = $1 AND p.status <> 'banned' AND u.status = 'active'`

	var (
		id, ownerID                int64
		title, description, status string
		slug, author               string
		province, unit             *string
		price                      *int64
		images                     []byte
		views, contacts            int
		createdAt                  time.Time
		isPro                      bool
		phone, line                *string
		catSlug, catName           *string
	)
	err := s.pool.QueryRow(r.Context(), sql, chi.URLParam(r, "slug")).Scan(
		&id, &slug, &title, &description, &province, &price, &unit, &images, &status,
		&views, &contacts, &createdAt, &ownerID, &author, &isPro, &phone, &line,
		&catSlug, &catName)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "ไม่พบประกาศนี้")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if status == "active" {
		s.recordView(r, id)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id": id, "slug": slug, "title": title, "description": description,
		"province": province, "price_from_satang": price, "price_unit": unit,
		"images": json.RawMessage(images), "author": author, "is_pro": isPro,
		"visible": status == "active", "created_at": createdAt,
		"view_count": views, "contact_count": contacts,
		"category_slug": catSlug, "category_name": catName,
		"is_owner":      userID(r) != 0 && userID(r) == ownerID,
		"contact_phone": phone, // NULL ถ้าเจ้าของเป็น free tier
		"contact_line":  line,
	}})
}

// เส้นเฉพาะสำหรับ sitemap.xml — คืนแค่ slug + updated_at ให้ได้ทีละมากๆ
//
// แยกจาก /posts เพราะเส้นนั้น cap limit ไว้ที่ 100 (กันคนดึงทั้ง DB ทีเดียว)
// ถ้าเอา /posts ไปทำ sitemap จะได้แค่ 100 URL แล้ว Google จะไม่เห็นประกาศที่เหลือเลย
func (s *Server) sitemapPosts(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 10000, 1, 50000)
	offset := queryInt(r, "offset", 0, 0, 5000000)

	rows, err := s.pool.Query(r.Context(), `
		SELECT p.slug, p.updated_at
		FROM posts p JOIN users u ON u.id = p.user_id
		WHERE p.status = 'active' AND u.status = 'active'
		  AND NOT p.is_demo          -- ประกาศตัวอย่างไม่ต้องให้ Google เก็บ
		ORDER BY p.id
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var slug string
		var updated time.Time
		if err := rows.Scan(&slug, &updated); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]any{"slug": slug, "updated_at": updated})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// นับ 1 วิว ต่อ 1 คน ต่อ 1 วัน — ตัวเลขนี้เอาไปโชว์ตอนขายค่าต่ออายุ ต้องเชื่อถือได้
func (s *Server) recordView(r *http.Request, postID int64) {
	tag, err := s.pool.Exec(r.Context(), `
		INSERT INTO post_views (post_id, viewer_key, view_date)
		VALUES ($1, $2, current_date) ON CONFLICT DO NOTHING`,
		postID, viewerKey(r))
	if err != nil || tag.RowsAffected() == 0 {
		return
	}
	_, _ = s.pool.Exec(r.Context(),
		`UPDATE posts SET view_count = view_count + 1 WHERE id = $1`, postID)
}

// ───────────────────────────────── ของฉัน (ต้อง login)

func (s *Server) myPosts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT p.id, p.slug, p.title, p.status, p.province, p.price_from_satang,
		       p.view_count, p.contact_count, p.bumped_at, p.created_at,
		       c.name_th
		FROM posts p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE p.user_id = $1 AND p.status <> 'banned'
		ORDER BY p.created_at DESC`, userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var (
			id                   int64
			slug, title, status  string
			province, catName    *string
			price                *int64
			views, contacts      int
			bumpedAt, createdAt  time.Time
		)
		if err := rows.Scan(&id, &slug, &title, &status, &province, &price,
			&views, &contacts, &bumpedAt, &createdAt, &catName); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]any{
			"id": id, "slug": slug, "title": title, "status": status,
			"province": province, "price_from_satang": price,
			"view_count": views, "contact_count": contacts,
			"bumped_at": bumpedAt, "created_at": createdAt, "category_name": catName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

type postInput struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	CategoryID   *int     `json:"category_id"`
	Province     string   `json:"province"`
	PriceFromTHB *float64 `json:"price_from_baht"`
	PriceUnit    string   `json:"price_unit"`
	Images       []string `json:"images"`
	ContactPhone string   `json:"contact_phone"`
	ContactLine  string   `json:"contact_line"`
	Status       string   `json:"status"` // active | hidden
}

func (in *postInput) validate(isPro bool) (string, bool) {
	in.Title = trim(in.Title, 120)
	if len([]rune(in.Title)) < 10 {
		return "ชื่อประกาศต้องยาวอย่างน้อย 10 ตัวอักษร", false
	}
	in.Description = trim(in.Description, 5000)
	if len([]rune(in.Description)) < 20 {
		return "รายละเอียดต้องยาวอย่างน้อย 20 ตัวอักษร", false
	}
	in.Province = trim(in.Province, 60)
	in.PriceUnit = trim(in.PriceUnit, 20)
	in.ContactPhone = trim(in.ContactPhone, 20)
	in.ContactLine = trim(in.ContactLine, 60)

	if in.PriceFromTHB != nil && (*in.PriceFromTHB < 0 || *in.PriceFromTHB > 10_000_000) {
		return "ราคาไม่ถูกต้อง", false
	}
	if n, max := len(in.Images), imageLimit(isPro); n > max {
		if isPro {
			return "อัปโหลดรูปได้สูงสุด 10 รูป", false
		}
		return "บัญชีฟรีใส่รูปได้ 1 รูป — อัปเกรดเป็น Pro เพื่อใส่ได้ถึง 10 รูป", false
	}
	for _, u := range in.Images {
		if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "/") {
			return "ลิงก์รูปภาพต้องขึ้นต้นด้วย https://", false
		}
	}
	if in.Status != "active" && in.Status != "hidden" {
		in.Status = "active"
	}
	return "", true
}

func (in postInput) satang() *int64 {
	if in.PriceFromTHB == nil {
		return nil
	}
	v := int64(*in.PriceFromTHB * 100)
	return &v
}

func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	var in postInput
	if !decodeJSON(w, r, &in) {
		return
	}

	uid := userID(r)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	// ล็อกแถว user ไว้ก่อนนับ ไม่งั้นกดสร้างรัวๆ พร้อมกันจะทะลุลิมิตได้
	var isPro bool
	if err := tx.QueryRow(r.Context(),
		`SELECT coalesce(pro_until > now(), false) FROM users WHERE id = $1 FOR UPDATE`,
		uid).Scan(&isPro); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if msg, ok := in.validate(isPro); !ok {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}

	var active int
	if err := tx.QueryRow(r.Context(),
		`SELECT count(*) FROM posts WHERE user_id = $1 AND status = 'active'`,
		uid).Scan(&active); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if in.Status == "active" && active >= postLimit(isPro) {
		if isPro {
			writeErr(w, http.StatusConflict, "ลงประกาศพร้อมกันได้สูงสุด 10 รายการ กรุณาปิดรายการเดิมก่อน")
		} else {
			writeErr(w, http.StatusConflict,
				"บัญชีฟรีลงประกาศได้ 1 รายการ — อัปเกรดเป็น Pro 300฿/เดือน เพื่อลงได้ถึง 10 รายการและขึ้นอันดับบนสุด")
		}
		return
	}

	imgs, _ := json.Marshal(in.Images)
	if in.Images == nil {
		imgs = []byte(`[]`)
	}

	var id int64
	err = tx.QueryRow(r.Context(), `
		INSERT INTO posts (user_id, category_id, title, slug, description, province,
		                   price_from_satang, price_unit, images, contact_phone,
		                   contact_line, status)
		VALUES ($1,$2,$3,'',$4,nullif($5,''),$6,nullif($7,''),$8,nullif($9,''),nullif($10,''),$11)
		RETURNING id`,
		uid, in.CategoryID, in.Title, in.Description, in.Province,
		in.satang(), in.PriceUnit, imgs, in.ContactPhone, in.ContactLine, in.Status).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// ต่อ id ท้าย slug -> unique แน่นอนและ URL คงที่ตลอดไปแม้แก้ชื่อทีหลัง
	slug := slugpkg.Make(in.Title) + "-" + strconv.FormatInt(id, 10)
	if _, err := tx.Exec(r.Context(),
		`UPDATE posts SET slug = $1 WHERE id = $2`, slug, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"id": id, "slug": slug}})
}

func (s *Server) updatePost(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "id ไม่ถูกต้อง")
		return
	}
	var in postInput
	if !decodeJSON(w, r, &in) {
		return
	}

	uid := userID(r)
	var isPro bool
	if err := s.pool.QueryRow(r.Context(),
		`SELECT coalesce(pro_until > now(), false) FROM users WHERE id = $1`, uid).Scan(&isPro); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msg, ok := in.validate(isPro); !ok {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}

	imgs, _ := json.Marshal(in.Images)
	if in.Images == nil {
		imgs = []byte(`[]`)
	}

	// full replace โดยตั้งใจ ไม่ใช่ partial patch — ฟอร์มแก้ไขส่งทุกฟิลด์เสมอ
	// ฟิลด์ไหนไม่ส่งมาจะถูกล้างเป็น NULL (เช่นไม่ส่ง contact_phone = ตั้งใจลบเบอร์ออก)
	//
	// WHERE user_id = $... คือด่านกันคนแก้โพสคนอื่น อย่าเช็คแค่ในโค้ด
	tag, err := s.pool.Exec(r.Context(), `
		UPDATE posts SET title = $1, description = $2, category_id = $3,
		       province = nullif($4,''), price_from_satang = $5, price_unit = nullif($6,''),
		       images = $7, contact_phone = nullif($8,''), contact_line = nullif($9,''),
		       status = $10, updated_at = now()
		WHERE id = $11 AND user_id = $12 AND status <> 'banned'`,
		in.Title, in.Description, in.CategoryID, in.Province, in.satang(), in.PriceUnit,
		imgs, in.ContactPhone, in.ContactLine, in.Status, id, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "ไม่พบประกาศนี้ในบัญชีของคุณ")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": "บันทึกแล้ว"})
}

// ปิดประกาศ = ซ่อน ไม่ลบจริง เพราะ URL ต้องอยู่ต่อเพื่อรักษาอันดับ SEO
func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "id ไม่ถูกต้อง")
		return
	}
	tag, err := s.pool.Exec(r.Context(),
		`UPDATE posts SET status = 'hidden', updated_at = now()
		 WHERE id = $1 AND user_id = $2 AND status <> 'banned'`, id, userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "ไม่พบประกาศนี้ในบัญชีของคุณ")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": "ปิดประกาศแล้ว"})
}

// ดัน bumped_at ให้ขึ้นบนสุดของฟีด — สิทธิ์ Pro เท่านั้น
func (s *Server) bumpPost(w http.ResponseWriter, r *http.Request) {
	id, ok := urlID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "id ไม่ถูกต้อง")
		return
	}
	tag, err := s.pool.Exec(r.Context(), `
		UPDATE posts p SET bumped_at = now(), updated_at = now()
		FROM users u
		WHERE p.id = $1 AND p.user_id = $2 AND u.id = p.user_id
		  AND p.status = 'active' AND u.pro_until > now()`, id, userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusForbidden, "ดันประกาศขึ้นบนได้เฉพาะสมาชิก Pro ที่ประกาศยังเปิดอยู่")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": "ดันประกาศขึ้นบนสุดแล้ว"})
}
