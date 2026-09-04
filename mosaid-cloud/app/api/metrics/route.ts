import { NextRequest, NextResponse } from "next/server";
import { createClient } from "@/lib/supabase/server";
import { checkAccess } from "@/lib/access";

export const runtime = "nodejs";
export const maxDuration = 30;

export async function GET(req: NextRequest) {
  if (!checkAccess(req)) return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  if (!process.env.NEXT_PUBLIC_SUPABASE_URL || !process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY)
    return NextResponse.json({ error: "setup-needed" }, { status: 501 });
  const supabase = await createClient();
  const [{ data: invoices }, { data: expenses }, { data: clients }] = await Promise.all([
    supabase.from("invoices").select("*"),
    supabase.from("expenses").select("*"),
    supabase.from("clients").select("id,status"),
  ]);
  const list = invoices ?? [];
  const unpaid = list.filter((i) => i.status === "sent").reduce((a, i) => a + (i.amount || 0), 0);
  const paid = list.filter((i) => i.status === "paid").reduce((a, i) => a + (i.amount || 0), 0);
  const draft = list.filter((i) => i.status === "draft").reduce((a, i) => a + (i.amount || 0), 0);
  const overdue = list
    .filter((i) => i.status === "sent" && i.due_date && i.due_date < new Date().toISOString())
    .reduce((a, i) => a + (i.amount || 0), 0);
  const expensesSum = (expenses ?? []).reduce((a, e) => a + (e.amount || 0), 0);

  return NextResponse.json({
    unpaid,
    paid,
    draft,
    overdue,
    expenses: expensesSum,
    active_clients: (clients ?? []).filter((c) => c.status === "active").length,
    net: paid - expensesSum,
  });
}
