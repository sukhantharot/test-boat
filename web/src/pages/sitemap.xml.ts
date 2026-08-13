import type { APIRoute } from 'astro';
import { apiGet, type Post } from '../lib/api';

export const prerender = false;

// sitemap เอาเฉพาะโพสที่ status='active' เท่านั้น
// โพสที่หมดอายุถูกถอดออกจากที่นี่ แต่ URL ยังเปิดอยู่พร้อม noindex (ดู services/[slug].astro)
export const GET: APIRoute = async ({ site }) => {
  const origin = (site ?? new URL('http://localhost:4321')).origin;

  let posts: Post[] = [];
  try {
    posts = (await apiGet<{ data: Post[] }>('/api/v1/posts?limit=50000')).data;
  } catch {
    posts = [];
  }

  const urls = [
    `<url><loc>${origin}/</loc><changefreq>daily</changefreq><priority>1.0</priority></url>`,
    ...posts.map(
      (p) =>
        `<url><loc>${origin}/services/${encodeURIComponent(p.slug)}</loc>` +
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
