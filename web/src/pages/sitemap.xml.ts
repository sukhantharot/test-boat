import type { APIRoute } from 'astro';
import { apiGet } from '../lib/api';

export const prerender = false;

type SitemapPost = { slug: string; updated_at: string };

// ใช้ /sitemap/posts ไม่ใช่ /posts เพราะ /posts cap limit ไว้ที่ 100
// (ถ้าใช้เส้นนั้น sitemap จะมีแค่ 100 URL ตลอดกาล = ประกาศที่เหลือ Google ไม่เห็น)
//
// เอาเฉพาะโพส status='active' โพสที่หมดอายุถูกถอดออกจากที่นี่
// แต่ URL ยังเปิดอยู่พร้อม noindex (ดู services/[slug].astro)
const PAGE = 10_000;
const MAX_URLS = 50_000; // ลิมิตของ sitemap protocol ต่อ 1 ไฟล์

const esc = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

export const GET: APIRoute = async ({ site }) => {
  const origin = (site ?? new URL('http://localhost:4321')).origin;

  const posts: SitemapPost[] = [];
  try {
    for (let offset = 0; offset < MAX_URLS; offset += PAGE) {
      const res = await apiGet<{ data: SitemapPost[] }>(
        `/api/v1/sitemap/posts?limit=${PAGE}&offset=${offset}`,
      );
      posts.push(...res.data);
      if (res.data.length < PAGE) break; // หน้าสุดท้ายแล้ว
    }
  } catch {
    // API ล่มก็ยังต้องคืน sitemap ที่ valid ไม่งั้น Google จะบันทึกว่า fetch error
  }

  const urls = [
    `<url><loc>${origin}/</loc><changefreq>daily</changefreq><priority>1.0</priority></url>`,
    ...posts.map(
      (p) =>
        `<url><loc>${esc(`${origin}/services/${encodeURI(p.slug)}`)}</loc>` +
        `<lastmod>${p.updated_at.slice(0, 10)}</lastmod>` +
        `<changefreq>weekly</changefreq><priority>0.8</priority></url>`,
    ),
  ].join('\n');

  return new Response(
    `<?xml version="1.0" encoding="UTF-8"?>\n` +
      `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls}\n</urlset>`,
    {
      headers: {
        'content-type': 'application/xml; charset=utf-8',
        'cache-control': 'public, max-age=3600',
      },
    },
  );
};
