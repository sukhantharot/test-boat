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
}

func Load() (Config, error) {
	c := Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL: dbURL(),
		Env:         getenv("APP_ENV", "development"),
		WebOrigin:   getenv("WEB_ORIGIN", "http://localhost:4321"),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL (หรือ DATABASE_PRIVATE_URL) ไม่ได้ตั้งค่า")
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
