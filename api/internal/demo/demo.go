// Package demo สร้างและลบข้อมูลตัวอย่างสำหรับสาธิตให้ลูกค้าดู
//
// แยกออกมาเป็น package เพราะเรียกได้ 2 ทาง:
//   - CLI (cmd/seeddemo) ตอนรันบนเครื่องตัวเอง
//   - endpoint ฝั่ง admin ตอนรันบน Railway ซึ่งเข้าถึง Postgres ได้เฉพาะจาก
//     ข้างใน private network เท่านั้น (ไม่ได้เปิด public access)
//
// ทุกแถวติดธง is_demo = true จึงลบทิ้งได้สะอาดโดยไม่แตะข้อมูลจริง
//
// ตั้งใจ "ไม่ใส่เบอร์โทร": เบอร์มือถือไทยที่สุ่มขึ้นมามีเจ้าของจริงเสมอ
// ถ้าคนที่มาดูเดโมกดโทรออก จะไปรบกวนคนที่ไม่เกี่ยวข้อง
package demo

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	slugpkg "boat/api/internal/slug"
)

const MaxPosts = 20000

type service struct {
	category string
	titles   []string
	details  []string
	minTHB   int
	maxTHB   int
}

var catalog = []service{
	{"graphic-design",
		[]string{"รับออกแบบโลโก้พร้อม CI Guideline", "ออกแบบแบนเนอร์โฆษณาออนไลน์",
			"รับทำ Packaging และฉลากสินค้า", "ออกแบบโปสเตอร์และสื่อสิ่งพิมพ์",
			"รับทำนามบัตรและหัวจดหมาย"},
		[]string{"ออกแบบตามบรีฟ แก้ไขได้จนกว่าจะพอใจ ส่งไฟล์ต้นฉบับ AI/PSD ครบ พร้อมไฟล์สำหรับพิมพ์",
			"ทำงานด้านกราฟิกมากว่า 6 ปี มีผลงานให้ดูก่อนตัดสินใจ คุยงานผ่าน LINE ได้ตลอด",
			"เน้นงานเรียบ อ่านง่าย ใช้ได้จริงกับทุกสื่อ ส่งงานตรงเวลา มีใบเสร็จให้"},
		1500, 25000},
	{"web-development",
		[]string{"รับทำเว็บไซต์ WordPress พร้อมระบบหลังบ้าน", "รับทำเว็บไซต์บริษัทแบบ Responsive",
			"รับทำระบบ E-commerce ครบวงจร", "รับแก้ไขและดูแลเว็บไซต์รายเดือน",
			"รับทำ Landing Page สำหรับยิงแอด"},
		[]string{"ทำเว็บให้โหลดเร็ว รองรับมือถือ ติดตั้ง SSL และตั้งค่า SEO พื้นฐานให้ครบ",
			"ส่งมอบพร้อมคู่มือใช้งาน สอนใช้หลังบ้านฟรี 1 ชั่วโมง รับประกันแก้บั๊ก 3 เดือน",
			"รับงานตั้งแต่เว็บเล็กถึงระบบสั่งซื้อ เชื่อมต่อ Payment Gateway ของไทยได้"},
		5000, 120000},
	{"marketing",
		[]string{"รับดูแลเพจ Facebook และยิงแอด", "วางแผนการตลาดออนไลน์ครบวงจร",
			"รับทำ SEO ให้ติดหน้าแรก Google", "รับดูแล TikTok และ Instagram",
			"รับเขียนคอนเทนต์ลงโซเชียล"},
		[]string{"วางแผนคอนเทนต์รายเดือน ออกแบบภาพประกอบ พร้อมรายงานผลทุกสัปดาห์",
			"ยิงแอดโดยดูจากงบและกลุ่มเป้าหมายจริง ปรับแคมเปญให้ต่อเนื่อง ไม่ปล่อยทิ้ง",
			"เน้นวัดผลได้จริง มีรายงานยอด engagement และต้นทุนต่อ lead ให้ทุกเดือน"},
		3000, 45000},
	{"writing",
		[]string{"รับเขียนบทความ SEO ภาษาไทย", "รับแปลเอกสารอังกฤษ-ไทย",
			"รับเขียนคำโฆษณาและสโลแกน", "รับพิสูจน์อักษรและตรวจงานเขียน",
			"รับเขียนสคริปต์คลิปวิดีโอ"},
		[]string{"บทความ 800-1500 คำ ค้นข้อมูลจากแหล่งจริง ไม่คัดลอก ตรวจซ้ำก่อนส่งทุกครั้ง",
			"งานด่วนภายใน 24 ชั่วโมงได้ คิดเพิ่มตามความเร่ง แจ้งราคาชัดเจนก่อนเริ่มงาน",
			"รับงานสายสุขภาพ การเงิน ท่องเที่ยว และไอที มีตัวอย่างงานให้ดูก่อน"},
		400, 8000},
	{"video",
		[]string{"รับตัดต่อวิดีโอสำหรับ YouTube", "รับตัดคลิปสั้น TikTok และ Reels",
			"รับทำ Motion Graphic และอินโฟกราฟิก", "รับถ่ายและตัดต่อวิดีโอพรีเซนต์บริษัท",
			"รับใส่ซับไตเติลและปรับแต่งเสียง"},
		[]string{"ตัดต่อพร้อมใส่ซับ เพลงประกอบ และปรับสี ส่งงานเป็นไฟล์ 4K แก้ไขได้ 3 ครั้ง",
			"มีอุปกรณ์ครบ รับงานทั้งในและนอกสถานที่ คุยรายละเอียดก่อนเริ่มงานทุกครั้ง",
			"เข้าใจจังหวะคลิปสั้น ทำให้คนดูจบ มีตัวอย่างผลงานหลายแนวให้เลือกดู"},
		800, 30000},
	{"photography",
		[]string{"รับถ่ายภาพสินค้าสำหรับขายออนไลน์", "รับถ่ายภาพอาหารและเครื่องดื่ม",
			"รับถ่ายภาพอีเวนต์และงานสัมมนา", "รับถ่ายภาพบุคคลและโปรไฟล์",
			"รับรีทัชและแต่งภาพสินค้า"},
		[]string{"มีสตูดิโอและอุปกรณ์ไฟครบ ถ่ายพื้นขาวหรือจัดฉากตามต้องการ รีทัชให้ทุกรูป",
			"ส่งไฟล์ความละเอียดสูงภายใน 3 วัน พร้อมไฟล์ย่อสำหรับลงโซเชียลให้ด้วย",
			"รับงานนอกสถานที่ทั่วประเทศ คิดค่าเดินทางตามจริง แจ้งล่วงหน้าได้"},
		1000, 20000},
	{"home-service",
		[]string{"รับล้างแอร์และติดตั้งแอร์บ้าน", "รับซ่อมและติดตั้งระบบไฟฟ้าในบ้าน",
			"รับซ่อมประปาและเปลี่ยนสุขภัณฑ์", "รับทาสีบ้านและซ่อมผนัง",
			"รับติดตั้งกล้องวงจรปิดพร้อมเดินสาย"},
		[]string{"ช่างมีประสบการณ์ตรง มีอุปกรณ์ครบ ประเมินหน้างานฟรี แจ้งราคาก่อนลงมือทุกครั้ง",
			"รับประกันงาน 6 เดือน ถ้ามีปัญหาเข้าไปแก้ให้ฟรี ติดต่อได้ทุกวัน",
			"ทำงานสะอาด เก็บงานเรียบร้อย มีใบเสร็จ ออกใบกำกับภาษีได้"},
		500, 35000},
	{"other",
		[]string{"รับทำบัญชีและยื่นภาษีสำหรับร้านค้า", "รับจดทะเบียนบริษัทและนิติบุคคล",
			"รับสอนพิเศษออนไลน์ตัวต่อตัว", "รับคีย์ข้อมูลและจัดการเอกสาร",
			"รับที่ปรึกษาด้านธุรกิจ SME"},
		[]string{"ดูแลตั้งแต่ต้นจนจบ อธิบายให้เข้าใจทุกขั้นตอน ไม่ต้องมีความรู้มาก่อน",
			"คิดราคาตามปริมาณงานจริง ไม่มีค่าใช้จ่ายแอบแฝง แจ้งทุกอย่างล่วงหน้า",
			"ทำงานตรงเวลา ติดต่อกลับเร็ว มีลูกค้าประจำแนะนำต่อได้"},
		800, 40000},
}

var provinces = []string{
	"กรุงเทพมหานคร", "นนทบุรี", "ปทุมธานี", "สมุทรปราการ", "เชียงใหม่", "เชียงราย",
	"ขอนแก่น", "นครราชสีมา", "อุดรธานี", "ชลบุรี", "ระยอง", "ภูเก็ต",
	"สุราษฎร์ธานี", "สงขลา", "นครศรีธรรมราช", "พิษณุโลก", "อยุธยา", "ลำปาง",
}

var firstNames = []string{
	"สมชาย", "สมหญิง", "ณัฐพงษ์", "ปวีณา", "ธนกร", "ศิริพร", "อนุชา", "กมลวรรณ",
	"พีรพล", "ชนิดา", "วรเมธ", "อรพรรณ", "ธีรภัทร", "สุนิสา", "กิตติศักดิ์", "พิมพ์ชนก",
}

var lastNames = []string{
	"ใจดี", "รักงาน", "ศรีสุข", "วงศ์ทอง", "บุญมี", "แสงทอง", "พูนสุข", "จันทร์เพ็ญ",
	"ทองแท้", "มณีรัตน์", "เจริญพร", "สุขสันต์",
}

type Result struct {
	Posts    int `json:"posts"`
	Users    int `json:"users"`
	ProUsers int `json:"pro_users"`
}

// Purge ลบข้อมูลตัวอย่างทั้งหมด
//
// posts/leads หายเองตาม ON DELETE CASCADE แต่ payments เป็น ON DELETE RESTRICT
// (ตั้งใจ เพราะประวัติการเงินของผู้ใช้จริงต้องลบไม่ได้) ถ้าแอดมินเคยกด "แถมวัน Pro"
// ให้ผู้ใช้ตัวอย่าง จะมีแถว comp ค้างอยู่แล้วบล็อกการลบทั้งหมด
// จึงต้องเก็บ payments ของผู้ใช้ตัวอย่างทิ้งก่อน — ซึ่งก็เป็นข้อมูลตัวอย่างเหมือนกัน
func Purge(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`DELETE FROM payments WHERE user_id IN (SELECT id FROM users WHERE is_demo)`); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM users WHERE is_demo`)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Seed สร้างประกาศตัวอย่าง n รายการ กระจายให้ผู้ใช้สมมติราว n/4 คน (20% เป็น Pro)
//
// ทั้งหมดอยู่ใน transaction เดียว: ถ้าพังกลางทางจะไม่เหลือข้อมูลค้างครึ่งๆ กลางๆ
func Seed(ctx context.Context, pool *pgxpool.Pool, n int, seed int64) (Result, error) {
	var out Result
	if n < 1 || n > MaxPosts {
		return out, fmt.Errorf("จำนวนต้องอยู่ระหว่าง 1 ถึง %d", MaxPosts)
	}
	rng := rand.New(rand.NewSource(seed))
	numUsers := n/4 + 1

	catIDs := map[string]int{}
	rows, err := pool.Query(ctx, `SELECT slug, id FROM categories`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var slug string
		var id int
		if err := rows.Scan(&slug, &id); err != nil {
			rows.Close()
			return out, err
		}
		catIDs[slug] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	if len(catIDs) == 0 {
		return out, fmt.Errorf("ยังไม่มีหมวดหมู่ในฐานข้อมูล — รัน migrate ก่อน")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// กันสร้างซ้ำซ้อน: ถ้ามี demo อยู่แล้วให้ต่อลำดับจากของเดิม
	var offset int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE is_demo`).Scan(&offset); err != nil {
		return out, err
	}

	// ── ผู้ใช้สมมติ ───────────────────────────────────────────────────────
	// อีเมลใช้โดเมน .invalid (RFC 2606) ซึ่งไม่มีวัน resolve ได้จริง
	// password_hash ไม่ใช่ bcrypt hash จึงล็อกอินเป็นบัญชีเดโมไม่ได้
	userIDs := make([]int64, 0, numUsers)
	for i := 1; i <= numUsers; i++ {
		seq := offset + i
		name := firstNames[rng.Intn(len(firstNames))] + " " + lastNames[rng.Intn(len(lastNames))]
		isPro := i%5 == 0
		var proUntil *time.Time
		if isPro {
			t := time.Now().Add(time.Duration(7+rng.Intn(60)) * 24 * time.Hour)
			proUntil = &t
			out.ProUsers++
		}

		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO users (provider, provider_uid, email, password_hash,
			                   display_name, pro_until, is_demo, created_at)
			VALUES ('local', $1, $2, 'demo-no-login', $3, $4, true, now() - make_interval(days => $5))
			RETURNING id`,
			fmt.Sprintf("demo-%d", seq),
			fmt.Sprintf("demo%d@example.invalid", seq),
			name, proUntil, rng.Intn(180)).Scan(&id)
		if err != nil {
			return out, fmt.Errorf("สร้างผู้ใช้ตัวอย่างที่ %d ไม่สำเร็จ: %w", seq, err)
		}
		userIDs = append(userIDs, id)
	}

	// ── ประกาศ ────────────────────────────────────────────────────────────
	batch := &pgx.Batch{}
	for i := 1; i <= n; i++ {
		ui := rng.Intn(len(userIDs))
		svc := catalog[rng.Intn(len(catalog))]

		title := svc.titles[rng.Intn(len(svc.titles))]
		detail := svc.details[rng.Intn(len(svc.details))]
		province := provinces[rng.Intn(len(provinces))]
		priceTHB := (svc.minTHB + rng.Intn(svc.maxTHB-svc.minTHB+1)) / 100 * 100
		ageMinutes := rng.Intn(90 * 24 * 60)

		// ใช้ตัวสร้าง slug ตัวเดียวกับของจริง เพื่อให้ URL ที่โชว์ลูกค้าเหมือนของจริงเป๊ะ
		// ต่อ -demoN ท้ายแทน id เพราะยังไม่รู้ id ตอนอยู่ใน batch
		postSlug := slugpkg.Make(title) + "-demo" + strconv.Itoa(offset*4+i)

		batch.Queue(`
			INSERT INTO posts (user_id, category_id, title, slug, description, province,
			                   price_from_satang, price_unit, images, contact_line,
			                   status, is_demo, bumped_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'job','[]'::jsonb,$8,'active',true,
			        now() - make_interval(mins => $9),
			        now() - make_interval(mins => $9),
			        now() - make_interval(mins => $9))`,
			userIDs[ui], catIDs[svc.category], title, postSlug,
			detail+"\n\n(ประกาศตัวอย่างสำหรับสาธิตระบบ)",
			province, int64(priceTHB)*100,
			fmt.Sprintf("demo-line-%03d", ui+1), ageMinutes)
	}

	br := tx.SendBatch(ctx, batch)
	for i := range n {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return out, fmt.Errorf("สร้างประกาศที่ %d ไม่สำเร็จ: %w", i+1, err)
		}
	}
	if err := br.Close(); err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}

	out.Posts = n
	out.Users = numUsers
	return out, nil
}
