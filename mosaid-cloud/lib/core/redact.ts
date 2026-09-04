/**
 * حجب الأسرار قبل تسجيلها في السجل/التدقيق/الواجهة.
 * لا نخزّن أبداً مفاتيح الخام: نستبدلها بالنمط.
 */
const PATTERNS: RegExp[] = [
  /(api[_-]?key|token|password|secret|authorization)\s*[:=]\s*\S+/gi,
  /AIza[0-9A-Za-z_\-]{30,}/g,          // Gemini keys
  /sk-[0-9A-Za-z_\-]{16,}/g,           // OpenAI keys
  /\d{5,}:[A-Za-z0-9_\-]{30,}/g,       // Telegram bot token
];

export function redact(input: string): string {
  let out = input;
  for (const re of PATTERNS) out = out.replace(re, "<REDACTED>");
  return out;
}

/** يعيد قائمة "نصوص تشبه سر" لرفض حفظها في الذاكرة (مطابق لـ memory.Secret). */
export function looksLikeSecret(input: string): boolean {
  return PATTERNS.some((re) => re.test(input));
}
