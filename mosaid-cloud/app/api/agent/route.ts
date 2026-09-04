import { NextRequest, NextResponse } from "next/server";
import { generateText, type Message } from "ai";
import { getModel } from "@/lib/ai/provider";
import { buildAgentTools } from "@/lib/agent/skills";
import { SYSTEM_PROMPT } from "@/lib/agent/systemPrompt";
import { createClient } from "@/lib/supabase/server";
import { checkAccess } from "@/lib/access";
import { checkSetup } from "@/lib/core/failclosed";
import { Audit } from "@/lib/core/audit";
import { redact } from "@/lib/core/redact";

export const runtime = "nodejs";
export const maxDuration = 30;

export async function POST(req: NextRequest) {
  if (!checkAccess(req)) return NextResponse.json({ error: "unauthorized" }, { status: 401 });

  const body: { messages?: Message[] } = await req.json().catch(() => ({}));
  if (!body?.messages?.length) return NextResponse.json({ error: "no-messages" }, { status: 400 });

  // Fail-closed: رفض فوري قبل أي اتصال لو الإعداد ناقص.
  const problems = checkSetup(process.env as Record<string, string | undefined>);
  if (problems.length)
    return NextResponse.json({ error: "setup-needed", problems }, { status: 501 });

  const supabase = await createClient();
  const model = getModel();
  const tools = buildAgentTools(supabase);

  try {
    const lastUser = body.messages[body.messages.length - 1]?.content || "";
    const result = await generateText({
      model,
      system: SYSTEM_PROMPT,
      messages: body.messages,
      tools,
      maxSteps: 8,
      temperature: 0.4,
    });

    const usedTools = (result.toolResults?.map((r) => r.toolName) ?? []) as string[];
    await Audit(supabase, {
      kind: "conversation",
      detail: { prompt: redact(String(lastUser)).slice(0, 200), toolsUsed: usedTools },
    });

    return NextResponse.json({ text: result.text, toolCalls: usedTools });
  } catch (err) {
    console.error("agent error", err);
    await Audit(supabase, { kind: "agent_error", decision: "error" }).catch(() => {});
    return NextResponse.json({ error: "agent-failed" }, { status: 500 });
  }
}
