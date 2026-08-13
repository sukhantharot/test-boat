package httpx

import (
	"net/http"

	"boat/api/internal/auth"
)

// requireUser / requireAdmin ใช้ cookie คนละใบ + secret คนละตัว
// token ฝั่ง user จึงเอามาใช้กับ /admin ไม่ได้เด็ดขาด แม้จะแก้ claims เอง
func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.identify(r, auth.CookieUser, s.cfg.JWTUserSecret, auth.AudienceUser)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบก่อน")
			return
		}
		next.ServeHTTP(w, withValue(r, ctxUserID, id))
	})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.identify(r, auth.CookieAdmin, s.cfg.JWTAdminSecret, auth.AudienceAdmin)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบผู้ดูแล")
			return
		}
		next.ServeHTTP(w, withValue(r, ctxAdminID, id))
	})
}

// ไม่บังคับ login แต่ถ้า login มาก็ผูก user id ไว้ให้ (ใช้ตอนนับ lead)
func (s *Server) optionalUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := s.identify(r, auth.CookieUser, s.cfg.JWTUserSecret, auth.AudienceUser); ok {
			r = withValue(r, ctxUserID, id)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) identify(r *http.Request, cookie string, secret []byte, audience string) (int64, bool) {
	c, err := r.Cookie(cookie)
	if err != nil || c.Value == "" {
		return 0, false
	}
	id, err := auth.Parse(secret, audience, c.Value)
	if err != nil {
		return 0, false
	}
	return id, true
}
