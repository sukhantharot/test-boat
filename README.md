# Boat — มาร์เก็ตเพลสฟรีแลนซ์ (Fastwork-style)

ฟรีแลนซ์โพสขายบริการตัวเอง · ผู้จ้างติดต่อตรง · เก็บค่าสมาชิก 300฿/30วัน · **free tier ถาวร**

| | Free (ถาวร) | Pro — 300฿/30วัน |
|---|---|---|
| จำนวนโพส | 1 | 10 |
| อันดับในฟีด | ล่าง | **บนสุดเสมอ** |
| ช่องทางติดต่อ | ซ่อน → ผ่านฟอร์ม | แสดงเบอร์/LINE ตรง |
| รูปภาพ | 1 | 10 |
| Bump ขึ้นบน | ✗ | ✓ |
| สถิติ view/lead | ✗ | ✓ |

> Free tier ถาวรทำให้เว็บ**ไม่ร้าง** และ**ไม่เสีย SEO** ตอนคนไม่จ่าย
> 300฿ จึงขาย "การมองเห็น" ไม่ใช่ "ค่าผ่านประตู" — ปิดการขายง่ายกว่ามาก

Subscription ผูกกับ **ผู้ใช้** (`users.pro_until`) ไม่ใช่รายโพส

---

## Stack

| ส่วน | เทคโนโลยี | เหตุผล |
|---|---|---|
| `web/` | Astro 5 SSR + adapter Node | SEO — server-render ทุกหน้า, JSON-LD, sitemap |
| `api/` | Go 1.24 + chi + pgx | binary เดียว ~15MB, RAM ต่ำ = ค่า Railway ถูก |
| DB | PostgreSQL 16 | เรื่องเงินต้องมี transaction จริง + รายงานหลังบ้านเขียน SQL จบใน 3 บรรทัด |
| ค้นหาไทย | `pg_trgm` | Postgres ตัดคำไทยไม่ได้ trigram พอสำหรับหลักหมื่นโพส ค่อยย้ายไป Meilisearch ทีหลัง |

---

## โครงสร้าง

```
api/
  cmd/api/          HTTP server
  cmd/migrate/      รันเป็น preDeployCommand
  cmd/cron/         Railway cron service (รันแล้ว exit)
  internal/         config, db, httpx, migrate
  migrations/       *.sql ฝังเข้า binary ด้วย go:embed
  Dockerfile        multi-stage -> distroless (build 3 binaries ใน image เดียว)
  railway.json      config ของ service `api`
  railway.cron.json config ของ service `cron`
web/
  src/pages/        index, services/[slug], sitemap.xml, healthz
  src/layouts/      Base.astro (canonical / og / noindex / JSON-LD)
  src/lib/api.ts    แยก INTERNAL_API_URL (SSR) กับ PUBLIC_API_URL (browser)
docker-compose.yml  Postgres สำหรับ dev
```

---

## รันบนเครื่องตัวเอง

```powershell
Copy-Item .env.example .env
docker compose up -d

go -C api run ./cmd/migrate     # สร้างตาราง + seed หมวดหมู่
go -C api run ./cmd/api         # http://localhost:8080/healthz

cd web; npm install; npm run dev # http://localhost:4321
```

---

## Deploy ขึ้น Railway

### 1. สร้าง project + Postgres

```powershell
railway login
railway init                 # ตั้งชื่อ project
railway add --database postgres
```

### 2. สร้าง 3 services จาก repo เดียวกัน

ใน Railway dashboard กด **New → GitHub Repo** เลือก repo นี้ **3 ครั้ง** แล้วตั้งค่าแต่ละตัว:

| Service | Root Directory | Config-as-code Path | Public Domain |
|---|---|---|---|
| `api` | `api` | `railway.json` | ✅ Generate |
| `cron` | `api` | `railway.cron.json` | ❌ ไม่ต้อง |
| `web` | `web` | `railway.json` | ✅ Generate + custom domain |

> `cron` ใช้ Dockerfile ตัวเดียวกับ `api` แค่เปลี่ยน `startCommand` เป็น `/app/cron`
> ไม่ต้อง build image แยก และ**ห้าม**ตั้ง healthcheck ให้ `cron` เพราะ process จบแล้ว exit

### 3. Variables

**api** และ **cron**
```
DATABASE_URL = ${{Postgres.DATABASE_URL}}
APP_ENV      = production
WEB_ORIGIN   = https://${{web.RAILWAY_PUBLIC_DOMAIN}}
```

**web**
```
INTERNAL_API_URL = http://${{api.RAILWAY_PRIVATE_DOMAIN}}:8080
PUBLIC_API_URL   = https://${{api.RAILWAY_PUBLIC_DOMAIN}}
PUBLIC_SITE_URL  = https://${{web.RAILWAY_PUBLIC_DOMAIN}}
```

อย่าตั้ง `PORT` เอง — Railway ฉีดให้

### 4. Deploy

```powershell
railway up
railway logs
```

`preDeployCommand` จะรัน `/app/migrate` ให้อัตโนมัติก่อนสลับเวอร์ชัน — migration พัง = ไม่ปล่อยขึ้น

---

## ⚠ กับดัก Railway ที่โค้ดนี้กันไว้แล้ว

**1. Private network เป็น IPv6 → ห้าม bind `0.0.0.0`**
สาเหตุอันดับหนึ่งของ 502 ระหว่าง service
- Go: `net.Listen("tcp", ":"+port)` → Go เปิด `[::]` แบบ dual-stack ให้เอง ([main.go](api/cmd/api/main.go))
- Node: `ENV HOST=::` ใน [web/Dockerfile](web/Dockerfile)
- bind `::` และ `0.0.0.0` พร้อมกันไม่ได้ ต้องเลือก dual-stack

**2. SSR กับ browser ใช้คนละ URL**
`*.railway.internal` เข้าถึงได้เฉพาะจากใน Railway — เอาไปใส่ในโค้ดฝั่ง browser จะ fetch ไม่ติด
[web/src/lib/api.ts](web/src/lib/api.ts) แยกไว้แล้ว: SSR ใช้ private (ไม่คิด egress), browser ใช้ public

**3. `PUBLIC_*` ของ Astro ถูก inline ตอน build ไม่ใช่ตอน run**
แก้ค่าใน dashboard เฉยๆ ไม่มีผล ต้อง **redeploy**

**4. `cronSchedule` เป็น UTC เสมอ**
`0 18 * * *` = ตี 1 ของวันถัดไปตามเวลาไทย

**5. Postgres connection limit**
มี 3 service ต่อพร้อมกัน `MaxConns` ตั้งไว้ 10/service แล้วใน [db.go](api/internal/db/db.go)

---

## ค่าใช้จ่ายโดยประมาณ

Railway คิดตามการใช้จริง (RAM × เวลา + vCPU × เวลา) — Hobby $5/เดือน มีเครดิตให้ $5

| Service | RAM คร่าวๆ |
|---|---|
| api (Go distroless) | ~30–60 MB |
| web (Node SSR) | ~150–250 MB |
| cron | รันวันละ ~10 วินาที ≈ ฟรี |
| Postgres | ~100–200 MB |

รวมประมาณ **$10–20/เดือน** ตอนทราฟฟิกยังน้อย

**ประหยัดเพิ่ม:** วาง Cloudflare หน้า custom domain → cache หน้า public, ได้ Turnstile กันสแปมฟรี, เก็บรูปที่ R2 (egress ฟรี) แทน Railway volume

---

## งานที่เหลือของ Phase 1

โครงนี้ deploy ขึ้นได้แล้ว (health check ผ่าน, migration รัน, ฟีดดึงข้อมูลจริง) ส่วนที่ยังเป็น TODO:

- [ ] **Auth** — LINE Login + Google OAuth (ไม่ต้องเขียนระบบ verify เอง แต่ได้ contact ที่เชื่อถือได้ ใช้ทวงค่าต่ออายุ)
- [ ] **CRUD โพส** + อัปโหลดรูปขึ้น R2 + บังคับลิมิต free 1 / pro 10
- [ ] **Leads** — `POST /posts/{id}/leads` บังคับ login ก่อนเปิดเบอร์ ⭠ ตัวเลขนี้คือสิ่งที่ใช้ปิดการขาย 300฿
- [ ] **Billing** — PromptPay QR (Omise/GB Prime Pay) + webhook → insert `payments` → `pro_until += 30 days`
  แนะนำ **prepaid expiry** ไม่ใช่ recurring billing เพราะคนไทยไม่ค่อยผูกบัตร
- [ ] **Admin** — JWT คนละ secret, cookie คนละชื่อ, subdomain แยก + วาง Cloudflare Access ทับอีกชั้น (ฟรี 50 users)
  4 หน้าพอ: dashboard ตัวเลข / users / posts (ต้องมีปุ่มต่ออายุให้ฟรี) / payments
- [ ] **LINE Messaging API** แจ้งเตือนต่ออายุ 7/3/1 วัน ([cron/main.go](api/cmd/cron/main.go) log ไว้แล้ว รอต่อท่อ)
- [ ] **Turnstile** ที่หน้าสมัคร/โพส + rate limit 3 โพส/วัน
- [ ] Seed โพสจริง 30–50 รายการด้วยมือ **ก่อนเปิดตัว** — อย่าเปิดตอนเว็บว่าง
