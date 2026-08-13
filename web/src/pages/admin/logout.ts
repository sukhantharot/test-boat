import type { APIRoute } from 'astro';
import { callApi } from '../../lib/backend';
import { rejectCrossSite } from '../../lib/csrf';

export const prerender = false;

export const POST: APIRoute = async ({ request }) => {
  const blocked = rejectCrossSite(request);
  if (blocked) return blocked;

  const res = await callApi(request, '/api/v1/admin/logout', { method: 'POST' });

  const headers = new Headers({ location: '/admin/login' });
  // ส่ง Set-Cookie ที่ล้าง cookie ต่อให้เบราว์เซอร์
  for (const c of res.setCookies) headers.append('set-cookie', c);

  return new Response(null, { status: 303, headers });
};
