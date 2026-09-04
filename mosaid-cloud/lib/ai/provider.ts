import { createGoogleGenerativeAI } from "@ai-sdk/google";
import { openai } from "@ai-sdk/openai";
import { anthropic } from "@ai-sdk/anthropic";

/**
 * مزوّد النموذج. الافتراضي: Gemini من خلال نُقطة OpenAI-compatible (مجاني).
 * عيّن MODEL_PROVIDER إلى openai/anthropic للتبديل، مع مفاتيحهما.
 * سياسة الإعداد: free-only؛ `limits.max_cost_usd` في هذه الصيغة السحابية
 * تقع على قاعدة "لا حساب دفع" — فالحدود المجانية تُطبَّق عبر مزوّد Google.
 */
export function getModel() {
  const provider = process.env.MODEL_PROVIDER || "google";

  if (provider === "openai") {
    return openai(process.env.OPENAI_MODEL || "gpt-4o-mini");
  }
  if (provider === "anthropic") {
    return anthropic(process.env.ANTHROPIC_MODEL || "claude-3-5-haiku-latest");
  }

  const google = createGoogleGenerativeAI({
    apiKey: process.env.GOOGLE_GENERATIVE_AI_API_KEY,
  });
  return google(process.env.GOOGLE_MODEL || "gemini-2.5-flash");
}
