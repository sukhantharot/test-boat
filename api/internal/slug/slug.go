// Package slug สร้าง URL slug ที่เก็บอักษรไทยไว้
//
// แยกออกมาเป็น package เพราะทั้ง httpx (ตอนผู้ใช้สร้างประกาศ) และ seeddemo
// ต้องได้ slug หน้าตาเดียวกัน ถ้าเขียนแยกกันสองที่เดี๋ยวหลุดจากกัน
package slug

import (
	"strings"
	"unicode"
)

// Make แปลงข้อความเป็น slug โดยตั้งใจไม่ทับศัพท์เป็นอังกฤษ
// เพราะคำไทยใน URL ช่วย SEO ภาษาไทยมากกว่า
// (เบราว์เซอร์ percent-encode ให้เอง แต่แสดงผลกลับมาเป็นไทย)
//
// ⚠ ต้องรับ Mn/Mc ด้วย ไม่ใช่แค่ IsLetter: สระบน-ล่างและวรรณยุกต์ไทย
// (ั ิ ี ุ ู ่ ้ ๊ ๋ ์) เป็น nonspacing mark ซึ่ง unicode.IsLetter คืน false
// ถ้าตกไปจะได้ "ร-บออกแบบโลโก-" แทน "รับออกแบบโลโก้"
func Make(s string) string {
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
