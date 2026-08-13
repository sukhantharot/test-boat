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
});
