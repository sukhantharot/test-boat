import type { APIRoute } from 'astro';

export const prerender = false;

// railway.json ของ service `web` ชี้ healthcheckPath มาที่นี่
export const GET: APIRoute = () =>
  new Response(JSON.stringify({ ok: true }), {
    headers: { 'content-type': 'application/json' },
  });
