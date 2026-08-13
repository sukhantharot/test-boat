// เรียก Go API จาก 2 ฝั่ง คนละ URL:
//
//   SSR (โค้ดที่รันบน server ของ Astro) -> INTERNAL_API_URL = http://api.railway.internal:8080
//        วิ่งใน private network ของ Railway: เร็วกว่า และ "ไม่คิดค่า egress"
//
//   Browser (island, ฟอร์ม, dashboard)  -> PUBLIC_API_URL   = https://api-xxx.up.railway.app
//        ต้องเป็น public domain เพราะ browser เข้า private network ไม่ได้
//
// ผิดพลาดตรงนี้บ่อยมาก: เอา .railway.internal ไปใส่ในโค้ดฝั่ง browser แล้วงงว่าทำไม fetch fail

// INTERNAL_API_URL ไม่ใช่ PUBLIC_ จึงอ่านตอน runtime ได้ ต้องอ่านจาก process.env
// (import.meta.env จะถูก inline ตอน build ซึ่งตอนนั้นค่ายังไม่มี)
export const INTERNAL_API_URL =
  process.env.INTERNAL_API_URL || import.meta.env.INTERNAL_API_URL || 'http://localhost:8080';

export const PUBLIC_API_URL = import.meta.env.PUBLIC_API_URL || 'http://localhost:8080';

export async function apiGet<T>(path: string, init?: RequestInit): Promise<T> {
  const url = `${INTERNAL_API_URL}${path}`;
  let res: Response;
  try {
    res = await fetch(url, {
      ...init,
      signal: AbortSignal.timeout(10_000),
      headers: { Accept: 'application/json', ...(init?.headers ?? {}) },
    });
  } catch (e) {
    // โผล่ใน Railway logs ของ service `web` -> บอกได้เลยว่าไปเรียก URL ไหนแล้วพังเพราะอะไร
    console.error(`[api] fetch failed ${url}:`, (e as Error).message);
    throw e;
  }
  if (!res.ok) {
    console.error(`[api] ${url} -> HTTP ${res.status}`);
    throw new Error(`API ${path} -> ${res.status}`);
  }
  return res.json() as Promise<T>;
}

export type Post = {
  id: number;
  slug: string;
  title: string;
  province: string | null;
  price_from_satang: number | null;
  price_unit: string | null;
  images: string[];
  bumped_at: string;
  author: string;
  is_pro: boolean;
  category_slug: string | null;
  category_name: string | null;
};

export type Category = { id: number; slug: string; name: string };

/** รูปแบบ response ของ endpoint ที่แบ่งหน้า — GET /api/v1/posts คืน meta มาด้วย */
export type Paged<T> = {
  data: T[];
  meta: { total: number; limit: number; offset: number };
};

export const baht = (satang: number | null) =>
  satang == null ? 'สอบถามราคา' : `฿${(satang / 100).toLocaleString('th-TH')}`;
