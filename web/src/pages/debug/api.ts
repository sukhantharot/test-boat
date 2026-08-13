import type { APIRoute } from 'astro';
import { INTERNAL_API_URL, PUBLIC_API_URL } from '../../lib/api';

export const prerender = false;

// หน้า diagnose ตอนต่อ Railway ใหม่ๆ — บอกว่า SSR เรียก API ที่ URL ไหนและพังเพราะอะไร
// ไม่มี secret หลุด (โชว์แค่ hostname:port) แต่พอ deploy นิ่งแล้วลบไฟล์นี้ทิ้งได้เลย
export const GET: APIRoute = async () => {
  const target = `${INTERNAL_API_URL}/healthz`;
  const started = performance.now();

  let result: Record<string, unknown>;
  try {
    const res = await fetch(target, { signal: AbortSignal.timeout(10_000) });
    result = {
      reachable: res.ok,
      status: res.status,
      body: await res.text().catch(() => null),
    };
  } catch (e) {
    result = {
      reachable: false,
      error: (e as Error).message,
      cause: String((e as Error).cause ?? ''),
    };
  }

  return new Response(
    JSON.stringify(
      {
        internal_api_url: INTERNAL_API_URL,   // SSR ใช้ตัวนี้
        public_api_url: PUBLIC_API_URL,       // browser ใช้ตัวนี้
        env_var_was_set: Boolean(process.env.INTERNAL_API_URL),
        target,
        elapsed_ms: Math.round(performance.now() - started),
        ...result,
        hint:
          'internal_api_url ต้องเป็น http://<ชื่อ service api>.railway.internal:<PORT ที่ api ฟังจริง> ' +
          'ถ้าเป็น localhost:8080 แปลว่ายังไม่ได้ตั้ง INTERNAL_API_URL ใน service web',
      },
      null,
      2,
    ),
    { headers: { 'content-type': 'application/json; charset=utf-8' } },
  );
};
