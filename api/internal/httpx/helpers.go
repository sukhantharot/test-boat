package httpx

import (
	"context"
	"encoding/json"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"
)

type ctxKey int

const (
	ctxUserID ctxKey = iota
	ctxAdminID
)

func userID(r *http.Request) int64 {
	v, _ := r.Context().Value(ctxUserID).(int64)
	return v
}

func adminID(r *http.Request) int64 {
	v, _ := r.Context().Value(ctxAdminID).(int64)
	return v
}

func withValue(r *http.Request, k ctxKey, v int64) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), k, v))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

// จำกัด body ไม่เกิน 1MB กัน request ใหญ่ผิดปกติมากินแรม
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง: "+err.Error())
		return false
	}
	return true
}

func urlID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

func queryInt(r *http.Request, key string, def, min, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// slug ที่เก็บอักษรไทยไว้ — ตั้งใจไม่ทับศัพท์เป็นอังกฤษ เพราะคำไทยใน URL
// ช่วยเรื่อง SEO ภาษาไทยมากกว่า (เบราว์เซอร์ percent-encode ให้เอง แต่แสดงผลเป็นไทย)
//
// ⚠ ต้องรับ Mn/Mc ด้วย ไม่ใช่แค่ IsLetter: สระบน-ล่างและวรรณยุกต์ไทย
// (ั ิ ี ุ ู ่ ้ ๊ ๋ ์) เป็น nonspacing mark ซึ่ง unicode.IsLetter คืน false
// ถ้าตกไปจะได้ "ร-บออกแบบโลโก-" แทน "รับออกแบบโลโก้" — พัง SEO ทั้งเว็บ
func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.In(r, unicode.Mn, unicode.Mc):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteRune('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if rs := []rune(out); len(rs) > 60 {
		out = strings.Trim(string(rs[:60]), "-")
	}
	if out == "" {
		out = "post"
	}
	return out
}

// คีย์นับวิวแบบไม่เก็บ IP ตรงๆ — พอกันคนกด refresh รัวจนตัวเลขเฟ้อ
// (ตัวเลขวิว/lead คือสิ่งที่เอาไปขายค่าต่ออายุ ถ้ามั่วก็ขายไม่ได้)
func viewerKey(r *http.Request) string {
	h := sha256.Sum256([]byte(clientIP(r) + "|" + r.UserAgent()))
	return hex.EncodeToString(h[:16])
}

func clientIP(r *http.Request) string {
	// chi middleware.RealIP เซ็ต RemoteAddr จาก X-Forwarded-For ให้แล้ว
	if host, _, ok := strings.Cut(r.RemoteAddr, ":"); ok {
		return host
	}
	return r.RemoteAddr
}

func trim(s string, max int) string {
	s = strings.TrimSpace(s)
	if rs := []rune(s); len(rs) > max {
		return string(rs[:max])
	}
	return s
}
