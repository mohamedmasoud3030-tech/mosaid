import type { SupabaseClient } from "@supabase/supabase-js";

/**
 * سجل تدقيق (نقل من internal/audit).
 * نقوم بتسجيل كل قرار/طلب/كتابة مهمة في audit_log.
 */
export async function Audit(
  supabase: SupabaseClient,
  entry: { kind: string; actor?: string; resource?: string; decision?: string; detail?: Record<string, unknown> }
): Promise<void> {
  try {
    await supabase.from("audit_log").insert({
      kind: entry.kind,
      actor: entry.actor ?? "owner",
      resource: entry.resource,
      decision: entry.decision,
      detail: entry.detail ?? {},
    });
  } catch {
    // سجل التدقيق لا يجب أن يوقف العمليات؛ نجح أو فشل بالفعل المعاملة.
  }
}
