-- 0001_init.sql
-- โมเดล: ฟรีแลนซ์โพสขายบริการตัวเอง (Fastwork-style)
-- Subscription ผูกกับ "ผู้ใช้" ไม่ใช่ "โพส"  -> users.pro_until
-- Free tier ถาวร: โพสได้ 1 ชิ้น, อันดับล่าง, ซ่อนเบอร์ติดต่อ
-- Pro 300฿/30วัน:  โพสได้ 10 ชิ้น, อันดับบน, แสดงเบอร์/LINE, bump ได้

CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ---------------------------------------------------------------- users
CREATE TABLE users (
    id            bigserial PRIMARY KEY,
    provider      text        NOT NULL CHECK (provider IN ('line', 'google')),
    provider_uid  text        NOT NULL,
    email         citext,
    phone         text,
    display_name  text        NOT NULL,
    avatar_url    text,
    line_user_id  text,                       -- สำหรับยิงแจ้งเตือนผ่าน LINE Messaging API
    pro_until     timestamptz,                -- NULL/อดีต = free tier
    status        text        NOT NULL DEFAULT 'active'
                              CHECK (status IN ('active', 'suspended')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_uid)
);
CREATE INDEX users_pro_until_idx ON users (pro_until) WHERE pro_until IS NOT NULL;

-- ----------------------------------------------------------- categories
CREATE TABLE categories (
    id         serial PRIMARY KEY,
    slug       text   NOT NULL UNIQUE,
    name_th    text   NOT NULL,
    name_en    text,
    parent_id  int    REFERENCES categories (id) ON DELETE SET NULL,
    sort_order int    NOT NULL DEFAULT 0
);

-- ---------------------------------------------------------------- posts
CREATE TABLE posts (
    id                bigserial PRIMARY KEY,
    user_id           bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    category_id       int         REFERENCES categories (id) ON DELETE SET NULL,
    title             text        NOT NULL,
    slug              text        NOT NULL,
    description       text        NOT NULL DEFAULT '',
    province          text,
    price_from_satang bigint,                 -- เก็บเป็นสตางค์ (integer) ห้ามใช้ float กับเงิน
    price_unit        text,                   -- 'job' | 'hour' | 'day' | 'piece'
    images            jsonb       NOT NULL DEFAULT '[]'::jsonb,
    contact_line      text,
    contact_phone     text,
    status            text        NOT NULL DEFAULT 'draft'
                                  CHECK (status IN ('draft', 'active', 'hidden', 'banned')),
    bumped_at         timestamptz NOT NULL DEFAULT now(),   -- pro กด bump ขึ้นบนได้
    view_count        int         NOT NULL DEFAULT 0,
    contact_count     int         NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (slug)
);

-- ฟีดหลัก: pro ขึ้นก่อน แล้วค่อยเรียงตาม bumped_at
CREATE INDEX posts_feed_idx      ON posts (bumped_at DESC) WHERE status = 'active';
CREATE INDEX posts_user_idx      ON posts (user_id, status);
CREATE INDEX posts_category_idx  ON posts (category_id, bumped_at DESC) WHERE status = 'active';
CREATE INDEX posts_province_idx  ON posts (province, bumped_at DESC) WHERE status = 'active';

-- ค้นหาภาษาไทย: Postgres tokenize ไทยไม่ได้ -> ใช้ trigram แทน (พอสำหรับหลักหมื่นโพส)
CREATE INDEX posts_title_trgm_idx ON posts USING gin (title gin_trgm_ops);
CREATE INDEX posts_desc_trgm_idx  ON posts USING gin (description gin_trgm_ops);

-- ------------------------------------------------------------- payments
-- Ledger แบบ append-only: ห้าม UPDATE ยอดเงินย้อนหลัง ออกแถวใหม่เสมอ
CREATE TABLE payments (
    id            bigserial PRIMARY KEY,
    user_id       bigint      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    amount_satang bigint      NOT NULL CHECK (amount_satang > 0),   -- 300฿ = 30000
    currency      text        NOT NULL DEFAULT 'THB',
    method        text        NOT NULL CHECK (method IN ('promptpay', 'card', 'manual_slip', 'comp')),
    provider      text,                                            -- 'omise' | 'gbprimepay' | 'manual'
    provider_ref  text,                                            -- idempotency key จาก payment gateway
    slip_url      text,
    days_granted  int         NOT NULL DEFAULT 30,
    status        text        NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'paid', 'failed', 'refunded')),
    paid_at       timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_ref)      -- กัน webhook ยิงซ้ำ -> ต่ออายุซ้อน
);
CREATE INDEX payments_user_idx ON payments (user_id, created_at DESC);
CREATE INDEX payments_paid_idx ON payments (paid_at DESC) WHERE status = 'paid';

-- ---------------------------------------------------------------- leads
-- ทุกครั้งที่มีคนกด "ติดต่อ" = 1 lead  ==  ตัวเลขที่ใช้ขายค่าต่ออายุ 300฿
CREATE TABLE leads (
    id             bigserial PRIMARY KEY,
    post_id        bigint      NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    viewer_user_id bigint      REFERENCES users (id) ON DELETE SET NULL,
    channel        text        NOT NULL CHECK (channel IN ('phone', 'line', 'form')),
    message        text,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX leads_post_idx ON leads (post_id, created_at DESC);

-- --------------------------------------------------------------- admins
-- แยกตารางจาก users โดยตั้งใจ: คนละ JWT secret, คนละ cookie, คนละ route
CREATE TABLE admins (
    id            bigserial PRIMARY KEY,
    email         citext      NOT NULL UNIQUE,
    password_hash text        NOT NULL,
    name          text        NOT NULL,
    role          text        NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'owner')),
    last_login_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id          bigserial PRIMARY KEY,
    admin_id    bigint      REFERENCES admins (id) ON DELETE SET NULL,
    action      text        NOT NULL,
    target_type text,
    target_id   bigint,
    meta        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_admin_idx ON audit_logs (admin_id, created_at DESC);

-- ----------------------------------------------------------- seed data
INSERT INTO categories (slug, name_th, name_en, sort_order) VALUES
    ('graphic-design',   'กราฟิกดีไซน์',      'Graphic Design',   10),
    ('web-development',  'เว็บและโปรแกรม',    'Web & Software',   20),
    ('marketing',        'การตลาดออนไลน์',    'Marketing',        30),
    ('writing',          'เขียนบทความ/แปล',   'Writing',          40),
    ('video',            'วิดีโอและตัดต่อ',   'Video',            50),
    ('photography',      'ถ่ายภาพ',           'Photography',      60),
    ('home-service',     'ช่าง/บริการในบ้าน', 'Home Service',     70),
    ('other',            'อื่นๆ',             'Other',            99);
