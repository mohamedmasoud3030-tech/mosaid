import { NextRequest, NextResponse } from "next/server";
import { createClient } from "@/lib/supabase/server";
import { checkAccess } from "@/lib/access";
import { remember, search, forget, addCandidate, approveCandidate } from "@/lib/core/memory";
import { checkSetup } from "@/lib/core/failclosed";

export const runtime = "nodejs";
export const maxDuration = 30;

/** GET ?action=list | search&q= | ... */
export async function GET(req: NextRequest) {
  if (!checkAccess(req)) return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  const problems = checkSetup(process.env as Record<string, string | undefined>);
  if (problems.length) return NextResponse.json({ error: "setup-needed", problems }, { status: 501 });
  const supabase = await createClient();
  const sp = req.nextUrl.searchParams;
  const action = sp.get("action") || "list";

  if (action === "search") {
    const q = sp.get("q") || "";
    const data = await search(supabase, q);
    return NextResponse.json(data);
  }
  const { data, error } = await supabase.from("memory_items").select("*").order("created_at", { ascending: false }).limit(50);
  if (error) return NextResponse.json({ error: error.message }, { status: 500 });
  return NextResponse.json(data);
}

/** POST { action: remember|forget|candidate|approve, id?, content? } */
export async function POST(req: NextRequest) {
  if (!checkAccess(req)) return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  const problems = checkSetup(process.env as Record<string, string | undefined>);
  if (problems.length) return NextResponse.json({ error: "setup-needed", problems }, { status: 501 });
  const supabase = await createClient();
  const body = await req.json();
  const { action } = body;
  try {
    if (action === "remember") return NextResponse.json(await remember(supabase, body.content));
    if (action === "forget") { await forget(supabase, body.id); return NextResponse.json({ ok: true }); }
    if (action === "candidate") return NextResponse.json(await addCandidate(supabase, body.content, body.source || "owner"));
    if (action === "approve") return NextResponse.json(await approveCandidate(supabase, body.candidate_id));
    return NextResponse.json({ error: "unknown-action" }, { status: 400 });
  } catch (e) {
    return NextResponse.json({ error: (e as Error).message }, { status: 400 });
  }
}
