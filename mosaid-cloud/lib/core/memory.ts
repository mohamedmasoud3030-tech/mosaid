import { createHash } from "crypto";
import type { SupabaseClient } from "@supabase/supabase-js";
import { looksLikeSecret } from "./redact";
import { Audit } from "./audit";

/**
 * ذاكرة، بنمط internal/memory في Mosaid:
 *  - رفض أي نص يشبه سر (لا نخزّن أبداً).
 *  - تُحفَظ كمرشّحة أولا، ثم تُوافق يدوياً (أو عبر `remember`) قبل أن تصبح عنصراً.
 *  - فهرس FTS (tsvector) للبحث.
 */

export async function addCandidate(
  supabase: SupabaseClient,
  content: string,
  source = "owner",
  confidence = 1
): Promise<{ id: string; state: string }> {
  const text = content.trim();
  if (!text) throw new Error("empty memory");
  if (looksLikeSecret(text)) throw new Error("secret-like memory denied");

  const { data, error } = await supabase
    .from("memory_candidates")
    .insert({ content: text, source, confidence, state: "pending" })
    .select()
    .single();
  if (error) throw new Error("candidate failed: " + error.message);
  return { id: (data as { id: string }).id, state: "pending" };
}

/** ترقية مرشّحة إلى عنصر فعلي + كتابة tsvector. */
export async function approveCandidate(
  supabase: SupabaseClient,
  candidateId: string
): Promise<{ id: string }> {
  const { data, error } = await supabase
    .from("memory_candidates")
    .select("*")
    .eq("id", candidateId)
    .eq("state", "pending")
    .single();
  if (error) throw new Error("candidate not found/pending");
  const c = data as { content: string; source: string; confidence: number; expires_at: string | null };

  const { data: item, error: insErr } = await supabase
    .from("memory_items")
    .insert({
      content: c.content,
      source: c.source,
      confidence: c.confidence,
      expires_at: c.expires_at,
    })
    .select()
    .single();
  if (insErr) throw new Error("memory insert failed: " + insErr.message);
  const mid = (item as { id: string }).id;

  // tsvector (Postgres to_tsvector، بسيط عربي/نصي)
  await supabase.rpc("memory_set_tsv", { memory_id: mid, content: c.content });

  await supabase.from("memory_candidates").update({ state: "approved" }).eq("id", candidateId);
  await Audit(supabase, { kind: "memory_approved", resource: c.source });
  return { id: mid };
}

/** حفظ فوري (مرشّحة ثم موافقة) — مطابق لـ Remember. */
export async function remember(
  supabase: SupabaseClient,
  content: string
): Promise<{ id: string }> {
  const cand = await addCandidate(supabase, content, "owner", 1);
  return approveCandidate(supabase, cand.id);
}

/** بحث FTS. */
export async function search(
  supabase: SupabaseClient,
  query: string,
  limit = 20
): Promise<{ id: string; content: string; source: string; confidence: number; created_at: string }[]> {
  const q = query.trim();
  if (!q) return [];
  const { data } = await supabase.rpc("memory_search", { query: q, max_results: Math.min(limit, 50) });
  return (data as any) ?? [];
}

/** حذف عنصر ذاكرة. */
export async function forget(supabase: SupabaseClient, id: string): Promise<void> {
  await supabase.from("memory_items").delete().eq("id", id);
  await Audit(supabase, { kind: "memory_forgotten", resource: id });
}

export function memoryDigest(content: string): string {
  return createHash("sha256").update(content).digest("hex");
}

/** طرح مرشّحة لِـ "المفيد/مستحق الحفظ" — بدون موافقة (العناصر السريعة). */
export async function offerCandidate(
  supabase: SupabaseClient,
  content: string,
  confidence = 0.5
): Promise<void> {
  try {
    await addCandidate(supabase, content, "agent", confidence);
  } catch {
    // تجاهل (مثلا: تشبه سر)
  }
}
