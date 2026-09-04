import { NextRequest, NextResponse } from "next/server";
import { createClient } from "@/lib/supabase/server";
import { checkAccess } from "@/lib/access";
import { checkSetup } from "@/lib/core/failclosed";

export const runtime = "nodejs";
export const maxDuration = 30;

/** /status — نقطة صحية مطابقة لـ internal/health. */
export async function GET(req: NextRequest) {
  if (!checkAccess(req)) return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  const problems = checkSetup(process.env as Record<string, string | undefined>);

  const base: Record<string, unknown> = {
    status: problems.length ? "degraded" : "running",
    version: "0.1.0",
    provider: process.env.MODEL_PROVIDER || "google",
    fail_closed: problems.length === 0,
  };

  if (problems.length) {
    base.setup_needed = problems;
    return NextResponse.json(base, { status: 200 });
  }

  try {
    const supabase = await createClient();
    const [{ error: dbErr }, { data: pending }] = await Promise.all([
      supabase.from("clients").select("id").limit(1),
      supabase.from("approval_requests").select("id").eq("state", "pending"),
    ]);
    base.database = dbErr ? "error" : "ok";
    base.approvals_pending = (pending ?? []).length;
    return NextResponse.json(base);
  } catch {
    base.database = "error";
    return NextResponse.json(base, { status: 200 });
  }
}
