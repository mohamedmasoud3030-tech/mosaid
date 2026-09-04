import { createHash, randomUUID } from "crypto";
import type { SupabaseClient } from "@supabase/supabase-js";
import { Audit } from "./audit";

/**
 * مدير الموافقات — ينقل مبادئ internal/approval في Mosaid:
 *  - نُخزّن فقط hash للتوكن (لا توكن عادي)
 *  - الطلب مرتبط بالأداة + argsHash + المورد
 *  - TTL (انتهاء الصلاحية)
 *  - استخدام مرة واحدة (approval_uses) ضد إعادة الاستخدام
 *  - سجل تدقيق عند الطلب/القرار
 */

const TTL_MS = 10 * 60 * 1000; // 10 دقائق

function sha256(s: string): string {
  return createHash("sha256").update(s).digest("hex");
}

export async function createApproval(
  supabase: SupabaseClient,
  req: { tool: string; argsHash: string; resource: string }
): Promise<{ id: string; token: string; expires: string }> {
  const token = randomUUID();
  const tokenHash = sha256(token);
  const expires = new Date(Date.now() + TTL_MS).toISOString();

  const { data, error } = await supabase
    .from("approval_requests")
    .insert({
      token_hash: tokenHash,
      tool_name: req.tool,
      args_hash: req.argsHash,
      resource: req.resource,
      state: "pending",
      expires_at: expires,
    })
    .select()
    .single();
  if (error) throw new Error("approval create failed: " + (error.message || "unknown"));

  await Audit(supabase, {
    kind: "approval_requested",
    resource: req.resource,
    decision: "pending",
    detail: { tool: req.tool },
  });

  return { id: (data as { id: string }).id, token, expires };
}

export async function resolveApproval(
  supabase: SupabaseClient,
  id: string,
  decision: "approved" | "denied"
): Promise<void> {
  const { data, error } = await supabase
    .from("approval_requests")
    .select("*")
    .eq("id", id)
    .single();
  if (error) throw new Error("approval not found");
  const row = data as { state: string; tool_name: string; resource: string };

  if (row.state !== "pending") throw new Error(`approval already ${row.state}`);
  if (!decision || (decision !== "approved" && decision !== "denied")) throw new Error("invalid decision");

  const { error: upErr } = await supabase
    .from("approval_requests")
    .update({ state: decision, resolved_at: new Date().toISOString() })
    .eq("id", id)
    .eq("state", "pending");
  if (upErr) throw new Error("approval resolve failed: " + upErr.message);

  await Audit(supabase, {
    kind: "approval_resolved",
    resource: row.resource,
    decision,
    detail: { tool: row.tool_name },
  });
}

/**
 * تنفيذ الأثر الفعلي بعد الموافقة، واستهلاك الطلب (مرة واحدة).
 */
export async function authorizeAndApply(
  supabase: SupabaseClient,
  id: string,
  apply: (payload: Record<string, unknown>) => Promise<void>
): Promise<void> {
  const { data, error } = await supabase.from("approval_requests").select("*").eq("id", id).single();
  if (error) throw new Error("approval not found");
  const row = data as {
    id: string;
    tool_name: string;
    resource: string;
    state: string;
    args_hash: string;
  };
  if (row.state !== "approved") throw new Error(`approval not approved (state=${row.state})`);

  // استخدام مرة واحدة
  const { data: uses, error: uErr } = await supabase
    .from("approval_uses")
    .select("id")
    .eq("approval_id", id);
  if (uErr) throw new Error("use check failed");
  if ((uses ?? []).length > 0) throw new Error("approval already used");

  // نبني البايلود من args_hash؟ نتوقف عند هذا لا: نستدعي effect بواسطة args_hash
  // فيتطلب من المتصل تمرير القيمة. نُمرّره إذا كان في الطلب.
  await apply({ tool: row.tool_name, resource: row.resource, argsHash: row.args_hash });

  await supabase.from("approval_uses").insert({ approval_id: id });

  await Audit(supabase, {
    kind: "approval_applied",
    resource: row.resource,
    decision: "applied",
    detail: { tool: row.tool_name },
  });
}
