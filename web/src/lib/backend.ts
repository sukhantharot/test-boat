import { INTERNAL_API_URL } from './api';

// เรียก Go API จากฝั่ง server พร้อมส่ง cookie ของผู้ใช้ต่อไปด้วย
//
// ทำไมต้องผ่าน Astro ไม่ยิงจากเบราว์เซอร์ตรงๆ:
// web กับ api อยู่คนละ subdomain ของ up.railway.app ซึ่งอยู่ใน Public Suffix List
// เบราว์เซอร์จึงถือว่าเป็น cross-site -> cookie ต้องเป็น SameSite=None
// ซึ่งโดนบล็อกหนักขึ้นเรื่อยๆ  พอวิ่งผ่าน Astro ทุกอย่างกลายเป็น first-party
// cookie เป็น SameSite=Lax ได้ตามปกติ และ api ไม่ต้องเปิด public domain ด้วยซ้ำ

export type ApiResult<T> = {
  ok: boolean;
  status: number;
  data: T | null;
  error: string | null;
};

export async function callApi<T>(
  request: Request,
  path: string,
  init: RequestInit = {},
): Promise<ApiResult<T> & { setCookies: string[] }> {
  const cookie = request.headers.get('cookie');
  const headers = new Headers(init.headers);
  headers.set('accept', 'application/json');
  if (cookie) headers.set('cookie', cookie);
  if (init.body && !headers.has('content-type')) {
    headers.set('content-type', 'application/json; charset=utf-8');
  }

  let res: Response;
  try {
    res = await fetch(`${INTERNAL_API_URL}${path}`, {
      ...init,
      headers,
      signal: AbortSignal.timeout(15_000),
    });
  } catch (e) {
    console.error(`[backend] ${path} failed:`, (e as Error).message);
    return { ok: false, status: 0, data: null, error: 'เชื่อมต่อระบบไม่ได้', setCookies: [] };
  }

  let body: any = null;
  try {
    body = await res.json();
  } catch {
    /* ไม่ใช่ JSON ก็ปล่อยเป็น null */
  }

  return {
    ok: res.ok,
    status: res.status,
    data: (body?.data ?? null) as T | null,
    error: body?.error ?? (res.ok ? null : `เกิดข้อผิดพลาด (${res.status})`),
    setCookies: res.headers.getSetCookie?.() ?? [],
  };
}

/** เหมือน callApi แต่คืน meta ของ endpoint ที่แบ่งหน้าด้วย */
export async function callApiPaged<T>(
  request: Request,
  path: string,
): Promise<ApiResult<T[]> & { meta: { total: number; limit: number; offset: number } }> {
  const cookie = request.headers.get('cookie');
  const headers = new Headers({ accept: 'application/json' });
  if (cookie) headers.set('cookie', cookie);

  const empty = { total: 0, limit: 0, offset: 0 };
  try {
    const res = await fetch(`${INTERNAL_API_URL}${path}`, {
      headers,
      signal: AbortSignal.timeout(15_000),
    });
    const body: any = await res.json().catch(() => null);
    return {
      ok: res.ok,
      status: res.status,
      data: body?.data ?? null,
      error: body?.error ?? (res.ok ? null : `เกิดข้อผิดพลาด (${res.status})`),
      meta: body?.meta ?? empty,
    };
  } catch (e) {
    console.error(`[backend] ${path} failed:`, (e as Error).message);
    return { ok: false, status: 0, data: null, error: 'เชื่อมต่อระบบไม่ได้', meta: empty };
  }
}

export type AdminInfo = { id: number; email: string; name: string; role: string };

/**
 * ด่านหน้าทุกหน้าในหลังบ้าน
 * คืน Response (redirect) เมื่อยังไม่ได้ล็อกอิน ให้เพจ `return` ค่านั้นออกไปตรงๆ
 *
 * เช็คกับ API จริงทุกครั้งแทนที่จะดูแค่ว่ามี cookie ไหม เพราะ cookie ปลอมได้
 * แต่ลายเซ็น JWT ปลอมไม่ได้ (และ secret ฝั่ง admin คนละตัวกับฝั่ง user)
 */
export async function requireAdmin(request: Request): Promise<AdminInfo | Response> {
  const res = await callApi<AdminInfo>(request, '/api/v1/admin/me');
  if (!res.ok || !res.data) {
    return new Response(null, { status: 302, headers: { location: '/admin/login' } });
  }
  return res.data;
}

/** สร้าง URL ใหม่โดยคงพารามิเตอร์เดิมไว้ ใช้กับลิงก์แบ่งหน้าและตัวกรอง */
export function pageHref(url: URL, patch: Record<string, string | number | null>): string {
  const sp = new URLSearchParams(url.search);
  for (const [k, v] of Object.entries(patch)) {
    if (v === null || v === '') sp.delete(k);
    else sp.set(k, String(v));
  }
  const qs = sp.toString();
  return qs ? `${url.pathname}?${qs}` : url.pathname;
}

// ── ตัวช่วยจัดรูปแบบสำหรับหลังบ้าน ──────────────────────────────────────

export const fmtInt = (n: number) => n.toLocaleString('th-TH');

export const fmtBaht = (satang: number) =>
  `฿${(satang / 100).toLocaleString('th-TH', { maximumFractionDigits: 0 })}`;

export const fmtDate = (iso: string | null) =>
  iso
    ? new Date(iso).toLocaleDateString('th-TH', {
        day: 'numeric',
        month: 'short',
        year: '2-digit',
      })
    : '—';

export const fmtDateTime = (iso: string | null) =>
  iso
    ? new Date(iso).toLocaleString('th-TH', {
        day: 'numeric',
        month: 'short',
        year: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
      })
    : '—';

/** เหลืออีกกี่วันถึงหมดอายุ Pro — ติดลบแปลว่าหมดแล้ว */
export function daysLeft(iso: string | null): number | null {
  if (!iso) return null;
  return Math.ceil((new Date(iso).getTime() - Date.now()) / 86_400_000);
}
