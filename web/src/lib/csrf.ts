/**
 * กัน CSRF สำหรับฟอร์มที่ POST เข้ามา
 *
 * เทียบ header `Origin` กับ `Host` ของ request เดียวกัน — ทั้งคู่มาจาก request
 * ตัวเดียวกัน จึงไม่ขึ้นกับ astro config, โดเมนที่ผูก, หรือ reverse proxy
 * (ตัวที่ Astro มีให้เทียบกับ request.url ซึ่ง adapter node ตัด port ทิ้ง
 *  และหลัง proxy ก็ยังเห็นเป็น http ทำให้ฟอร์ม same-origin โดน 403 หมด)
 *
 * เว็บที่โจมตีจะมี Origin เป็นโดเมนตัวเอง ซึ่งไม่มีทางตรงกับ Host ของเรา
 *
 * ชั้นป้องกันจริงอีกชั้นคือ cookie เป็น SameSite=Lax อยู่แล้ว เบราว์เซอร์จึง
 * ไม่แนบ session ไปกับ POST ข้ามเว็บตั้งแต่แรก ตัวนี้เป็นเกราะชั้นที่สอง
 */
export function isSameOrigin(request: Request): boolean {
  const origin = request.headers.get('origin');
  const host = request.headers.get('host');
  if (!origin || !host) return false;

  try {
    return new URL(origin).host === host;
  } catch {
    return false;
  }
}

/** คืน Response 403 ถ้าไม่ใช่ same-origin — เพจเรียกแล้ว `return` ค่านั้นออกไป */
export function rejectCrossSite(request: Request): Response | null {
  if (isSameOrigin(request)) return null;
  return new Response('คำขอนี้ไม่ได้มาจากเว็บไซต์นี้', { status: 403 });
}
