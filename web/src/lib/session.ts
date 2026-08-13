// เรียก Go API จากฝั่ง server ของ Astro พร้อมแนบ cookie ของ request ที่เข้ามา
//
// ทำไมต้องมีไฟล์นี้ (อ่านคู่กับ api/internal/auth/auth.go):
//   cookie session ออกโดย Go API และตั้ง SameSite=Lax
//   ถ้าให้ browser ยิง API ข้าม domain ตรงๆ (web-xxx.up.railway.app -> api-xxx.up.railway.app)
//   cookie จะเป็น third-party -> Lax ไม่ถูกส่งไปกับ cross-site request = login ไม่ติด
//   และตั้ง Domain=.up.railway.app ก็ไม่ได้ เพราะอยู่ใน Public Suffix List
//
//   วิธีแก้: ทุกอย่างวิ่งผ่าน server ของ Astro แล้ว relay Set-Cookie ต่อให้ browser
//   cookie จึงถูกตั้งบน "โดเมนของเว็บ" = first-party = Lax ใช้ได้ตามที่ auth.go ตั้งใจ

import { INTERNAL_API_URL } from './api';

/** ชื่อ cookie ต้องตรงกับ auth.CookieUser ใน api/internal/auth/auth.go */
export const USER_COOKIE = 'boat_user';

export type Me = {
  id: number;
  email: string;
  display_name: string;
  phone: string | null;
  is_pro: boolean;
  pro_until: string | null;
  active_posts: number;
  post_limit: number;
  total_leads: number;
};

type ApiRequestInit = {
  method?: string;
  /** ส่งเป็น JSON — API ใช้ DisallowUnknownFields() ห้ามใส่ field เกินที่ struct รับ */
  body?: unknown;
  /** ค่า header `cookie` ของ request ที่เข้ามา (Astro.request.headers.get('cookie')) */
  cookie?: string | null;
  /** ต่อให้ผู้ใช้ปลายทางเห็นเป็น IP ของตัวเอง ไม่ใช่ IP ของ service web
      สำคัญกับ viewerKey() ที่ใช้นับ view/lead แบบ 1 คน 1 วัน */
  forwardedFor?: string | null;
  userAgent?: string | null;
  timeoutMs?: number;
};

/** ยิง API แล้วคืน Response ดิบ — ผู้เรียกต้องอ่าน status/Set-Cookie เองได้ */
export async function apiRequest(path: string, init: ApiRequestInit = {}): Promise<Response> {
  const { method = 'GET', body, cookie, forwardedFor, userAgent, timeoutMs = 10_000 } = init;

  const headers = new Headers({ Accept: 'application/json' });
  if (body !== undefined) headers.set('Content-Type', 'application/json');
  if (cookie) headers.set('Cookie', cookie);
  if (forwardedFor) headers.set('X-Forwarded-For', forwardedFor);
  if (userAgent) headers.set('User-Agent', userAgent);

  return fetch(`${INTERNAL_API_URL}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(timeoutMs),
    // ห้าม follow redirect เอง — ต้องส่งต่อให้ browser ตัดสินใจ
    redirect: 'manual',
  });
}

/** ข้อความ error ที่ API ส่งมา (ภาษาไทยพร้อมโชว์ผู้ใช้อยู่แล้ว) */
export async function apiError(res: Response, fallback = 'เกิดข้อผิดพลาด กรุณาลองใหม่'): Promise<string> {
  try {
    const j = (await res.json()) as { error?: string };
    return j.error?.trim() || fallback;
  } catch {
    return fallback;
  }
}

/**
 * ย้าย Set-Cookie จาก response ของ API มาใส่ response ที่จะส่งกลับ browser
 *
 * ต้องใช้ getSetCookie() ไม่ใช่ get('set-cookie') เพราะ Set-Cookie มีได้หลายใบ
 * แล้ว get() จะเอามาต่อกันด้วย ", " ซึ่งพัง (วันที่ใน Expires มี comma อยู่ข้างใน)
 */
export function relaySetCookie(from: Response, to: Headers): void {
  // getSetCookie() มีใน undici ของ Node 18.14+ แต่ TS DOM lib รุ่นเก่าไม่รู้จัก
  const h = from.headers as Headers & { getSetCookie?: () => string[] };
  const cookies =
    typeof h.getSetCookie === 'function'
      ? h.getSetCookie()
      : [h.get('set-cookie')].filter((v): v is string => !!v);
  for (const c of cookies) to.append('set-cookie', c);
}

/** redirect ที่พา Set-Cookie ไปด้วย — Astro.redirect() สร้าง Response ใหม่ จะทำ cookie หาย */
export function redirectWithCookies(location: string, from: Response, status = 303): Response {
  const headers = new Headers({ Location: location });
  relaySetCookie(from, headers);
  return new Response(null, { status, headers });
}

/**
 * ใครกำลัง login อยู่ — คืน null ถ้าไม่ได้ login
 *
 * ไม่มี cookie ก็ไม่ต้องยิง API เลย: ผู้เข้าชมส่วนใหญ่ (รวม Googlebot) ไม่ได้ login
 * ยิงทุก request ทุกหน้าคือ request ที่เสียเปล่า
 */
export async function getMe(cookie: string | null): Promise<Me | null> {
  if (!cookie || !cookie.includes(`${USER_COOKIE}=`)) return null;
  try {
    const res = await apiRequest('/api/v1/me', { cookie, timeoutMs: 5_000 });
    if (!res.ok) return null;
    return ((await res.json()) as { data: Me }).data;
  } catch {
    // API ล่มไม่ควรทำให้ทุกหน้าพัง — แค่แสดงเป็น "ยังไม่ login"
    return null;
  }
}

/** อ่านข้อมูลผู้ใช้จาก Astro global ในหน้า/คอมโพเนนต์เดียวบรรทัด */
export function requestCookie(request: Request): string | null {
  return request.headers.get('cookie');
}
