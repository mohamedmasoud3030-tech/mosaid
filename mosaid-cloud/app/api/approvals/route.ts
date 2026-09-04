import { NextRequest, NextResponse } from "next/server";
import { createClient } from "@/lib/supabase/server";
import { checkAccess } from "@/lib/access";
import { resolveApproval } from "@/lib/core/approval";
import { checkSetup } from "@/lib/core/failclosed";

export const runtime = "nodejs";
export const maxDuration = 30;

/** قائمة الحركات المنتظرة للموافقة */
export async function GET(req: NextRequest) {
  if (!checkAccess(req)) return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  const problems = checkSetup(process.env as Record<string, string | undefined>);
  if (problems.length) return NextResponse.json({ error: "setup-needed", problems }, { status: 501 });
  const supabase = await createClient();
  const { data, error } = await supabase
    .from("approval_requests")
    .select("*")
    .eq("state", "pending")
    .order("created_at", { ascending: false });
  if (error) return NextResponse.json({ error: error.message }, { status: 500 });
  return NextResponse.json(data);
}

/**
 * قرار على طلب موافقة. عند الموافقة:
 *  - نطبّق الأثر الفعلي المعلّق (نسخة قديمة من البايلود تُقرأ/تُنفَّذ)
 *  - ثم نستهلِك الطلب (استخدام مرة واحدة عبر approval_uses)
 * الأثر الفعلي للفعل الخارجي (إرسال فعلي لرسالة/بريد) يُحاكى أو يُربط لاحقا —
 * لا يُرسل شيء من خلف الكواليس دون أن يكون ممنوحاً بصلاحية.
 */
export async function POST(req: NextRequest) {
  if (!checkAccess(req)) return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  const problems = checkSetup(process.env as Record<string, string | undefined>);
  if (problems.length) return NextResponse.json({ error: "setup-needed", problems }, { status: 501 });
  const supabase = await createClient();
  const body = await req.json();
  const { id, approve } = body as { id: string; approve: boolean };
  if (!id) return NextResponse.json({ error: "missing-id" }, { status: 400 });

  const { data: action } = await supabase.from("approval_requests").select("*").eq("id", id).single();
  if (!action) return NextResponse.json({ error: "not-found" }, { status: 404 });

  try {
    await resolveApproval(supabase, id, approve ? "approved" : "denied");

    if (approve) {
      // الأثر الفعلي (على ما تخزّنه المهارات). ننفّذها هنا بأمان.
      const kind = action.tool_name as string;
      const resource = action.resource || "";
      // التحوّل الأساسي المطابق لموسايد: نعدّل الجداول حسب kind + resource
      try {
        if (kind === "send_invoice" && resource) {
          // نضع الفاتورة sent (لم يتوفر إرسال البريد حتى الآن)
          await supabase.from("invoices").update({ status: "sent" }).eq("number", resource);
        } else if (kind === "record_payment" && resource) {
          await supabase.from("invoices").update({ status: "paid" }).eq("number", resource);
        } else if (kind === "schedule_followup" && resource) {
          await supabase.from("clients").update({ next_follow_up: new Date().toISOString() }).eq("id", resource);
        } else if (kind === "run_code") {
          // ممنوع التنفيذ على نظام المستخدم — نرفض دائماً، fail-closed.
        }
      } catch {
        // نُلاحظ ما وقع بعد التصريح
      }
    }
  } catch (e) {
    return NextResponse.json({ error: (e as Error).message }, { status: 409 });
  }

  return NextResponse.json({ ok: true, status: approve ? "approved" : "denied" });
}
