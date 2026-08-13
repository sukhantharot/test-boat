// Package auth จัดการ JWT + password hash
//
// เจตนา: ฝั่ง user กับฝั่ง admin แยกขาดจากกันโดยสิ้นเชิง
//   - คนละ secret  -> token ของ user เอาไปใช้ฝั่ง admin ไม่ได้แม้จะปลอม claims
//   - คนละ audience -> ต่อให้ secret หลุดตัวหนึ่ง อีกฝั่งยังไม่พัง
//   - คนละ cookie   -> login ทั้งสองฝั่งพร้อมกันในเบราว์เซอร์เดียวได้ ไม่ทับกัน
package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	AudienceUser  = "user"
	AudienceAdmin = "admin"

	CookieUser  = "boat_user"
	CookieAdmin = "boat_admin"

	TokenTTL = 30 * 24 * time.Hour
)

var ErrInvalidToken = errors.New("token ไม่ถูกต้องหรือหมดอายุ")

func Sign(secret []byte, audience string, subjectID int64) (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(subjectID, 10),
		Audience:  jwt.ClaimStrings{audience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
	})
	return tok.SignedString(secret)
}

func Parse(secret []byte, audience, token string) (int64, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(token, &claims,
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("signing method ไม่ถูกต้อง: %v", t.Header["alg"])
			}
			return secret, nil
		},
		jwt.WithAudience(audience),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		return 0, ErrInvalidToken
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return id, nil
}

func SetCookie(w http.ResponseWriter, name, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		// Lax พอ เพราะ Astro proxy ทำให้ทุกอย่างเป็น first-party
		// (ถ้ายิง API ข้าม domain ตรงๆ จะต้องใช้ None ซึ่งเบราว์เซอร์บล็อกหนักขึ้นเรื่อยๆ)
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(TokenTTL.Seconds()),
	})
}

func ClearCookie(w http.ResponseWriter, name string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
