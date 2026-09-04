/**
 * حارس دخول بسيط لمحوّل الـ API.
 * إذا عيّنت AGENT_ACCESS_CODE في الرابط، يصبح على الأداة رمز للدخول
 * (يُرسَل في ترويسة x-access-code). إذا لم يُعيّن، يُتجاوز (مفيد للتطوير المحلي
 * قبل رفع Vercel).
 */
export function checkAccess(req: Request): boolean {
  const code = process.env.AGENT_ACCESS_CODE;
  if (!code) return true;
  const provided = req.headers.get("x-access-code");
  if (!provided) return false;
  return provided === code;
}
