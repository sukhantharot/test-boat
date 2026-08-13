import { defineConfig } from 'astro/config';
import node from '@astrojs/node';

// output: 'server' = SSR ทุกหน้า แล้วค่อยเลือก prerender เป็นรายหน้า
// (ใส่ `export const prerender = true` ในไฟล์ .astro) — หน้า landing/หมวดหมู่ควร prerender
//
// หมายเหตุ: บล็อค `server` ข้างล่างมีผลเฉพาะ `astro dev` / `astro preview`
// ตอนรันจริง adapter standalone จะอ่าน HOST/PORT จาก env (ตั้งไว้ใน Dockerfile แล้ว)
export default defineConfig({
  output: 'server',
  adapter: node({ mode: 'standalone' }),
  site: process.env.PUBLIC_SITE_URL || 'http://localhost:4321',
  server: { host: true, port: 4321 },
  build: { inlineStylesheets: 'auto' },

  // ปิด CSRF check ของ Astro แล้วเช็คเองใน src/lib/csrf.ts
  //
  // ของ Astro เทียบ header Origin กับ origin ที่ได้จาก request.url
  // แต่ adapter node สร้าง request.url โดย "ตัด port ทิ้ง" (ได้ http://localhost
  // ทั้งที่ Host คือ localhost:4321) และหลัง reverse proxy ของ Railway ก็ยัง
  // เห็น protocol เป็น http ทั้งที่ผู้ใช้เข้ามาทาง https
  // ผลคือฟอร์มทุกอันโดน 403 แม้เป็น same-origin จริงๆ
  //
  // ตัวที่เขียนเองเทียบ Origin กับ Host ของ request เดียวกัน จึงไม่ขึ้นกับ
  // config หรือพฤติกรรมของ proxy เลย
  security: { checkOrigin: false },
});
