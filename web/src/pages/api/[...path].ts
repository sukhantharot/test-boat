import type { APIRoute } from 'astro';
import { INTERNAL_API_URL } from '../../lib/api';
import { relaySetCookie } from '../../lib/session';

export const prerender = false;

// ─────────────────────────────────────────────────────────────────────────
// Proxy จาก browser -> Go API ผ่าน server ของ Astro
//
// นี่คือชิ้นที่ comment ใน api/internal/auth/auth.go พูดถึง:
//   "Lax พอ เพราะ Astro proxy ทำให้ทุกอย่างเป็น first-party"
//
// browser เรียก /api/v1/... บนโดเมนของเว็บ (same-origin) แล้วไฟล์นี้ยิงต่อเข้า
// private network ของ Railway  ผลคือ:
//   - Set-Cookie ถูกตั้งบนโดเมนเว็บ = first-party -> SameSite=Lax ทำงาน
//   - ไม่ต้องใช้ SameSite=None ซึ่ง Safari/Chrome บล็อกหนักขึ้นเรื่อยๆ
//   - ไม่เสีย egress เพราะขาที่วิ่งจริงอยู่ใน private network
//
// path ที่เข้ามาคือส่วนหลัง /api/ เช่น browser เรียก /api/v1/auth/login
// -> params.path = "v1/auth/login" -> ยิงต่อที่ ${INTERNAL_API_URL}/api/v1/auth/login
// (ใช้ path เดียวกับ API จริง จะได้ไม่ต้องจำ mapping สองชุด)
// ─────────────────────────────────────────────────────────────────────────

// ส่งต่อเฉพาะ header ที่จำเป็น — ห้ามส่ง host/origin/referer ของเว็บเข้าไป
// และห้ามส่ง accept-encoding เพราะ fetch จะ decode body ให้แล้ว
// ถ้าส่งต่อ content-encoding ไปด้วย browser จะพยายาม decode ซ้ำแล้วพัง
const FORWARD_REQUEST_HEADERS = ['content-type', 'accept', 'cookie', 'user-agent'];
const FORWARD_RESPONSE_HEADERS = ['content-type', 'cache-control'];

/** กัน path traversal ไม่ให้หลุดออกไปนอก /api ของ API */
function safePath(raw: string | undefined): string | null {
  if (!raw) return null;
  const p = raw.replace(/^\/+/, '');
  if (p.includes('..') || p.includes('\\')) return null;
  return p;
}

export const ALL: APIRoute = async ({ params, request, clientAddress }) => {
  const path = safePath(params.path);
  if (path === null) {
    return json(400, { error: 'path ไม่ถูกต้อง' });
  }

  const search = new URL(request.url).search;
  const target = `${INTERNAL_API_URL}/api/${path}${search}`;

  const headers = new Headers();
  for (const name of FORWARD_REQUEST_HEADERS) {
    const v = request.headers.get(name);
    if (v) headers.set(name, v);
  }

  // ให้ middleware.RealIP ของ chi เห็น IP ของผู้ใช้จริง ไม่ใช่ IP ของ service web
  // ไม่งั้น viewerKey() จะมองว่าทุกคนคือคนเดียวกัน แล้วยอด view/lead จะเพี้ยน
  const existing = request.headers.get('x-forwarded-for');
  headers.set('x-forwarded-for', existing ? `${existing}, ${clientAddress}` : clientAddress);

  // GET/HEAD ห้ามมี body ไม่งั้น undici จะ throw
  const hasBody = request.method !== 'GET' && request.method !== 'HEAD';

  let upstream: Response;
  try {
    upstream = await fetch(target, {
      method: request.method,
      headers,
      body: hasBody ? await request.arrayBuffer() : undefined,
      signal: AbortSignal.timeout(15_000),
      redirect: 'manual',
    });
  } catch (e) {
    console.error(`[proxy] ${request.method} ${target}:`, (e as Error).message);
    return json(502, { error: 'ติดต่อเซิร์ฟเวอร์ไม่ได้ กรุณาลองใหม่' });
  }

  const out = new Headers();
  for (const name of FORWARD_RESPONSE_HEADERS) {
    const v = upstream.headers.get(name);
    if (v) out.set(name, v);
  }
  // หัวใจของไฟล์นี้ — cookie จาก API ต้องถึง browser ไม่งั้น login ไม่ติด
  relaySetCookie(upstream, out);

  // อย่า cache คำตอบของ API ที่ผูกกับ session ไว้ที่ CDN/browser
  out.set('cache-control', upstream.headers.get('cache-control') ?? 'no-store');

  return new Response(upstream.body, { status: upstream.status, headers: out });
};

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
