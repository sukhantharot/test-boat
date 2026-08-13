-- 0002_auth.sql
-- Phase 1 ไม่ verify อะไรเลยตามที่ตกลงไว้ -> สมัครด้วย email+password ใช้ได้ทันที
-- เพิ่ม provider 'local' ไว้ข้างๆ line/google เพื่อให้ต่อ OAuth ทีหลังได้โดยไม่ต้องแก้ schema

ALTER TABLE users DROP CONSTRAINT users_provider_check;
ALTER TABLE users ADD CONSTRAINT users_provider_check
    CHECK (provider IN ('line', 'google', 'local'));

ALTER TABLE users ADD COLUMN password_hash text;

-- สมัครด้วยอีเมลได้คนละ 1 บัญชี (citext -> ไม่สนตัวพิมพ์เล็กใหญ่)
CREATE UNIQUE INDEX users_email_unique_idx ON users (email) WHERE email IS NOT NULL;

-- ผู้ใช้ 'local' ต้องมีทั้ง email และ password_hash เสมอ
ALTER TABLE users ADD CONSTRAINT users_local_needs_password
    CHECK (provider <> 'local' OR (email IS NOT NULL AND password_hash IS NOT NULL));

-- นับยอดวิวแบบ unique ต่อวัน กัน refresh รัวแล้วตัวเลขเฟ้อจนใช้ขายต่ออายุไม่ได้
CREATE TABLE post_views (
    post_id    bigint NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    viewer_key text   NOT NULL,           -- hash ของ IP+UA
    view_date  date   NOT NULL,
    PRIMARY KEY (post_id, viewer_key, view_date)
);
