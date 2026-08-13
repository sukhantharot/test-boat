-- 0003_demo_flag.sql
-- ธงแยก "ข้อมูลตัวอย่างสำหรับโชว์ลูกค้า" ออกจากประกาศจริง
--
-- จำเป็นเพราะประกาศตัวอย่างอยู่บนเว็บ production ที่ Google เข้ามาเก็บได้
-- ถ้าปล่อยให้ index ทั้ง 1,000 อัน Google จะมองเป็น thin/spam content
-- แล้วลดคะแนนทั้งโดเมน ซึ่งกู้คืนยากมาก
--
-- ธงนี้ทำ 3 อย่าง: ใส่ noindex, ตัดออกจาก sitemap, และลบทิ้งทีเดียวได้

ALTER TABLE users ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE posts ADD COLUMN is_demo boolean NOT NULL DEFAULT false;

CREATE INDEX users_demo_idx ON users (is_demo) WHERE is_demo;
CREATE INDEX posts_demo_idx ON posts (is_demo) WHERE is_demo;
