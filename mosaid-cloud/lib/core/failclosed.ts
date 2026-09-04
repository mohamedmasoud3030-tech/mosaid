import type { SupabaseClient } from "@supabase/supabase-js";

/**
 * فحص "fail-closed" لإعدادات النشر.
 * أي إعداد أساسي ناقص/غير صالح => الوكيل يرفض العمل بدل أن يَفشل في منتصف الطريق.
 * مطابق لروح `verify-config.sh` في Mosaid.
 */
export type SetupViolation = { key: string; problem: string };

export function checkSetup(env: Record<string, string | undefined>): SetupViolation[] {
  const problems: SetupViolation[] = [];

  if (!env.NEXT_PUBLIC_SUPABASE_URL) problems.push({ key: "NEXT_PUBLIC_SUPABASE_URL", problem: "missing" });
  if (!env.NEXT_PUBLIC_SUPABASE_ANON_KEY) problems.push({ key: "NEXT_PUBLIC_SUPABASE_ANON_KEY", problem: "missing" });

  const provider = env.MODEL_PROVIDER || "google";
  if (provider === "google" && !env.GOOGLE_GENERATIVE_AI_API_KEY)
    problems.push({ key: "GOOGLE_GENERATIVE_AI_API_KEY", problem: "missing (Gemini free tier)" });
  if (provider === "openai" && !env.OPENAI_API_KEY) problems.push({ key: "OPENAI_API_KEY", problem: "missing" });
  if (provider === "anthropic" && !env.ANTHROPIC_API_KEY) problems.push({ key: "ANTHROPIC_API_KEY", problem: "missing" });
  if (!["google", "openai", "anthropic"].includes(provider)) problems.push({ key: "MODEL_PROVIDER", problem: `unknown '${provider}'` });

  return problems;
}

/** التكلفة الحدية — قفل أمان لعدم التحول إلى الفوترة أثناء العمل المجاني. */
export const MAX_COST_TRIPWIRE_USD = 0.01;

export function assertReady(supabase: SupabaseClient): void {
  if (!process.env.NEXT_PUBLIC_SUPABASE_URL) throw new Error("fail-closed: MISSING SUPABASE_URL");
  if (!process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY) throw new Error("fail-closed: MISSING SUPABASE_ANON_KEY");
}
