// เรียก Go API จาก 2 ฝั่ง คนละ URL:
//
//   SSR (โค้ดที่รันบน server ของ Astro) -> INTERNAL_API_URL = http://api.railway.internal:8080
//        วิ่งใน private network ของ Railway: เร็วกว่า และ "ไม่คิดค่า egress"
//
//   Browser (island, ฟอร์ม, dashboard)  -> PUBLIC_API_URL   = https://api-xxx.up.railway.app
//        ต้องเป็น public domain เพราะ browser เข้า private network ไม่ได้
//
// ผิดพลาดตรงนี้บ่อยมาก: เอา .railway.internal ไปใส่ในโค้ดฝั่ง browser แล้วงงว่าทำไม fetch fail

const INTERNAL = import.meta.env.INTERNAL_API_URL || 'http://localhost:8080';
export const PUBLIC_API_URL = import.meta.env.PUBLIC_API_URL || 'http://localhost:8080';

export async function apiGet<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${INTERNAL}${path}`, {
    ...init,
    headers: { Accept: 'application/json', ...(init?.headers ?? {}) },
  });
  if (!res.ok) throw new Error(`API ${path} -> ${res.status}`);
  return res.json() as Promise<T>;
}

export type Post = {
  id: number;
  slug: string;
  title: string;
  province: string | null;
  price_from_satang: number | null;
  author: string;
  is_pro: boolean;
};

export const baht = (satang: number | null) =>
  satang == null ? 'สอบถามราคา' : `฿${(satang / 100).toLocaleString('th-TH')}`;
