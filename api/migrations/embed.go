package migrations

import "embed"

// ฝัง .sql เข้าไปใน binary เลย จะได้ deploy ไฟล์เดียวจบ ไม่ต้อง mount อะไรบน Railway
//
//go:embed *.sql
var FS embed.FS
