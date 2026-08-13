package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port        string // Railway ฉีด PORT ให้เอง
	DatabaseURL string
	Env         string
	WebOrigin   string // origin ของ Astro สำหรับ CORS

	JWTUserSecret  []byte
	JWTAdminSecret []byte

	// ใช้ seed admin คนแรกตอนรัน migrate (ดู cmd/migrate)
	AdminEmail    string
	AdminPassword string
}

func (c Config) IsProd() bool { return c.Env == "production" }

// cookie ต้องเป็น Secure บน production แต่ dev รัน http ธรรมดา เลยต้องปิด
func (c Config) CookieSecure() bool { return c.IsProd() }

func Load() (Config, error) {
	c := Config{
		Port:           getenv("PORT", "8080"),
		DatabaseURL:    dbURL(),
		Env:            getenv("APP_ENV", "development"),
		WebOrigin:      getenv("WEB_ORIGIN", "http://localhost:4321"),
		JWTUserSecret:  []byte(os.Getenv("JWT_USER_SECRET")),
		JWTAdminSecret: []byte(os.Getenv("JWT_ADMIN_SECRET")),
		AdminEmail:     strings.ToLower(getenv("ADMIN_EMAIL", "")),
		AdminPassword:  os.Getenv("ADMIN_PASSWORD"),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL (หรือ DATABASE_PRIVATE_URL) ไม่ได้ตั้งค่า")
	}

	// dev ให้รันได้เลยไม่ต้องตั้งอะไร แต่ production ต้องบังคับ ไม่งั้นใครก็ปลอม token ได้
	if len(c.JWTUserSecret) == 0 {
		if c.IsProd() {
			return c, fmt.Errorf("JWT_USER_SECRET ไม่ได้ตั้งค่า")
		}
		c.JWTUserSecret = []byte("dev-user-secret-ห้ามใช้บน-production")
	}
	if len(c.JWTAdminSecret) == 0 {
		if c.IsProd() {
			return c, fmt.Errorf("JWT_ADMIN_SECRET ไม่ได้ตั้งค่า")
		}
		c.JWTAdminSecret = []byte("dev-admin-secret-ห้ามใช้บน-production")
	}
	// secret เดียวกันทั้งสองฝั่ง = การแยก user/admin ไม่มีความหมาย
	if string(c.JWTUserSecret) == string(c.JWTAdminSecret) {
		return c, fmt.Errorf("JWT_USER_SECRET กับ JWT_ADMIN_SECRET ต้องไม่ซ้ำกัน")
	}

	return c, nil
}

// บน Railway ให้ผูกกับ ${{Postgres.DATABASE_URL}} ซึ่งวิ่งผ่าน private network
// (ฟรี ไม่คิด egress) ส่วน DATABASE_PUBLIC_URL ใช้เฉพาะตอนต่อจากเครื่องตัวเอง
func dbURL() string {
	for _, k := range []string{"DATABASE_PRIVATE_URL", "DATABASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
