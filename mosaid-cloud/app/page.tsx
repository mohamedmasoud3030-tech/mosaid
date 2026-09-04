"use client";

import { useEffect, useRef, useState } from "react";
import {
  Send,
  Bot,
  User,
  KeyRound,
  Wrench,
  Check,
  X,
  Wallet,
  Loader2,
  BarChart3,
} from "lucide-react";

type Message = { id: string; role: "user" | "assistant"; text: string; tools?: string[] };
type Approval = {
  id: string;
  tool_name: string;
  resource: string;
  state: string;
  created_at: string;
  token_hash?: string;
};
type Metrics = {
  unpaid: number;
  paid: number;
  draft: number;
  overdue: number;
  expenses: number;
  active_clients: number;
  net: number;
};

const EMPTY: Metrics = {
  unpaid: 0,
  paid: 0,
  draft: 0,
  overdue: 0,
  expenses: 0,
  active_clients: 0,
  net: 0,
};

export default function Home() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [metrics, setMetrics] = useState<Metrics>(EMPTY);
  const [showAccess, setShowAccess] = useState(false);
  const [accessCode, setAccessCode] = useState("");
  const [needAccess, setNeedAccess] = useState(false);
  const [setupNeeded, setSetupNeeded] = useState(false);
  const [usedTools, setUsedTools] = useState<Record<string, number>>({});
  const scrollRef = useRef<HTMLDivElement>(null);

  // ترويسة الطلب مع رمز الدخول الممكن
  const headers = (): HeadersInit => {
    const h: Record<string, string> = { "Content-Type": "application/json" };
    if (accessCode) h["x-access-code"] = accessCode;
    return h;
  };

  const loadData = async () => {
    try {
      const [m, a] = await Promise.all([
        fetch("/api/metrics", { headers: headers() }),
        fetch("/api/approvals", { headers: headers() }),
      ]);
      if (m.status === 401 || a.status === 401) {
        setNeedAccess(true);
        setShowAccess(true);
        return;
      }
      if (m.status === 501 || a.status === 501) {
        setSetupNeeded(true);
        return;
      }
      setMetrics(await m.json());
      setApprovals(await a.json());
    } catch (e) {
      console.error(e);
    }
  };

  useEffect(() => {
    loadData();
  }, [accessCode]);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [messages, loading]);

  const send = async () => {
    const text = input.trim();
    if (!text || loading) return;
    const userMsg: Message = { id: crypto.randomUUID(), role: "user", text };
    const next = [...messages, userMsg];
    setMessages(next);
    setInput("");
    setLoading(true);
    try {
      const res = await fetch("/api/agent", {
        method: "POST",
        headers: headers(),
        body: JSON.stringify({
          messages: next.map((m) => ({ role: m.role, content: m.text })),
        }),
      });
      if (res.status === 401) {
        setNeedAccess(true);
        setShowAccess(true);
        setLoading(false);
        return;
      }
      if (res.status === 501) {
        setSetupNeeded(true);
        setMessages((prev) => [
          ...prev,
          {
            id: crypto.randomUUID(),
            role: "assistant",
            text: "لحد دلوقتي التطبيق لسه موش موصول بقاعدة البيانات. افتح README وعيّن متغيّرات Supabase ثم أعد النشر على Vercel.",
          },
        ]);
        setLoading(false);
        return;
      }
      const data = await res.json();
      const tools = (data.toolCalls || []) as string[];
      setMessages((prev) => [
        ...prev,
        { id: crypto.randomUUID(), role: "assistant", text: data.text || "…", tools },
      ]);
      const count = { ...usedTools };
      tools.forEach((t) => (count[t] = (count[t] || 0) + 1));
      setUsedTools(count);
      loadData();
    } catch (e) {
      console.error(e);
      setMessages((prev) => [
        ...prev,
        { id: crypto.randomUUID(), role: "assistant", text: "حصل خطأ. حاول تاني." },
      ]);
    } finally {
      setLoading(false);
    }
  };

  const labelOf = (tool: string): string => {
    const map: Record<string, string> = {
      send_invoice: "إرسال فاتورة",
      send_message: "إرسال رسالة",
      schedule_followup: "جدولة متابعة",
      record_payment: "تسجيل دفع",
      run_code: "تنفيذ كود",
    };
    return map[tool] || tool;
  };

  const decide = async (id: string, approve: boolean) => {
    await fetch("/api/approvals", {
      method: "POST",
      headers: headers(),
      body: JSON.stringify({ id, approve }),
    });
    loadData();
  };

  const card = (label: string, value: number, color = "text-slate-50") => (
    <div className="bg-white/5 border border-white/10 rounded-xl p-3">
      <div className="text-[11px] text-slate-400">{label}</div>
      <div className={`text-lg font-bold ${color}`}>{value.toLocaleString()}</div>
    </div>
  );

  return (
    <div className="min-h-screen text-slate-100 flex flex-col">
      {/* الشريط العلوي */}
      <header className="border-b border-white/10 bg-white/5 backdrop-blur px-4 py-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Bot className="h-5 w-5 text-emerald-400" />
          <h1 className="font-bold text-lg">رحلتي — وكيلك الشخصي</h1>
          <span className="text-[11px] bg-emerald-500/20 text-emerald-300 px-2 py-0.5 rounded-full">
            مجاني · supabase · vercel
          </span>
        </div>
        <div className="flex items-center gap-2">
        <button
          onClick={() => setShowAccess(true)}
          className="text-xs flex items-center gap-1.5 border border-white/15 rounded-lg px-3 py-1.5 hover:bg-white/10"
        >
          <KeyRound className="h-3.5 w-3.5" /> رمز الدخول
        </button>
        </div>
      </header>

      {setupNeeded && (
        <div className="bg-amber-500/15 border-b border-amber-500/30 px-4 py-2.5 text-sm text-amber-200">
          ⚙️ التطبيق جاهز للعرض لكنه لسه محتاج إعدادات Supabase. راجع{" "}
          <b>README.md</b> → عيّن <code>NEXT_PUBLIC_SUPABASE_URL</code> و{" "}
          <code>NEXT_PUBLIC_SUPABASE_ANON_KEY</code> → أعد النشر على Vercel ليعمل الوكيل.
        </div>
      )}

      <div className="flex flex-1 overflow-hidden">
        {/* الشريط الجانبي (المؤشرات + الموافقات) */}
        <aside className="hidden lg:flex flex-col gap-4 w-80 border-l border-white/10 bg-white/[0.03] p-4 overflow-y-auto">
          <div>
            <h2 className="text-sm font-semibold text-slate-300 flex items-center gap-2 mb-3">
              <BarChart3 className="h-4 w-4 text-emerald-400" /> نبذة عن عملك
            </h2>
            <div className="grid grid-cols-2 gap-2">
              {card("مستحق (sent)", metrics.unpaid)}
              {card("مُدفوع", metrics.paid, "text-emerald-400")}
              {card("متأخر", metrics.overdue, "text-rose-400")}
              {card("مصاريف", metrics.expenses)}
            </div>
            <div className="mt-3 grid grid-cols-2 gap-2">
              {card("عملاء نشطون", metrics.active_clients)}
              {card("صافي", metrics.net, "text-emerald-300")}
            </div>
          </div>

          <div>
            <h2 className="text-sm font-semibold text-slate-300 flex items-center gap-2 mb-3">
              <Wallet className="h-4 w-4 text-amber-400" /> في انتظار موافقتك
            </h2>
            {approvals.length === 0 ? (
              <p className="text-xs text-slate-500">
                لا توجد حركات تنتظر موافقتك. الوكيل يسألك قبل أي فعل خارجي.
              </p>
            ) : (
              <div className="space-y-2">
                {approvals.map((a) => (
                  <div key={a.id} className="bg-white/5 border border-white/10 rounded-xl p-3">
                    <p className="text-sm font-semibold">{labelOf(a.tool_name)}</p>
                    <p className="text-xs text-slate-400 mt-0.5">{a.resource}</p>
                    <div className="mt-2 flex gap-2">
                      <button
                        onClick={() => decide(a.id, true)}
                        className="flex items-center gap-1 text-xs bg-emerald-500/80 rounded-lg px-2 py-1 hover:bg-emerald-500"
                      >
                        <Check className="h-3.5 w-3.5" /> أوافق
                      </button>
                      <button
                        onClick={() => decide(a.id, false)}
                        className="flex items-center gap-1 text-xs bg-white/10 rounded-lg px-2 py-1 hover:bg-white/20"
                      >
                        <X className="h-3.5 w-3.5" /> أرفض
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </aside>

        {/* منطقة الشات */}
        <main className="flex-1 flex flex-col">
          <div ref={scrollRef} className="flex-1 overflow-y-auto px-4 py-6 space-y-4 max-w-3xl mx-auto w-full">
            {messages.length === 0 && (
              <div className="text-center mt-20">
                <Bot className="h-12 w-12 mx-auto text-emerald-400/70 mb-3" />
                <p className="text-slate-300">أهلاً! في مجال عملك كمستقل، أكتب لك أي حاجة:</p>
                <div className="mt-4 flex flex-wrap justify-center gap-2">
                  {[
                    "مين عميلي النشط؟",
                    "أضف عميل جديد",
                    "كم مستحق عليّ المتأخر؟",
                    "جهّز فاتورة",
                    "لخّص أرقام الشهر",
                  ].map((s) => (
                    <button
                      key={s}
                      onClick={() => setInput(s)}
                      className="text-xs bg-white/5 border border-white/10 rounded-full px-3 py-1.5 hover:bg-white/10"
                    >
                      {s}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {messages.map((m) => (
              <div key={m.id} className={`flex gap-2 ${m.role === "user" ? "justify-end" : ""}`}>
                {m.role === "assistant" && (
                  <div className="h-7 w-7 rounded-full bg-emerald-500/20 flex items-center justify-center shrink-0">
                    <Bot className="h-4 w-4 text-emerald-300" />
                  </div>
                )}
                <div
                  className={`max-w-[80%] rounded-2xl px-4 py-2.5 text-[15px] leading-relaxed whitespace-pre-wrap ${
                    m.role === "user"
                      ? "bg-emerald-600 text-white"
                      : "bg-white/10 text-slate-100"
                  }`}
                >
                  {m.text}
                  {m.tools && m.tools.length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-1">
                      {m.tools.map((t) => (
                        <span
                          key={t}
                          className="text-[10px] flex items-center gap-1 bg-emerald-500/20 text-emerald-200 rounded px-1.5 py-0.5"
                        >
                          <Wrench className="h-3 w-3" /> {t}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
                {m.role === "user" && (
                  <div className="h-7 w-7 rounded-full bg-slate-600 flex items-center justify-center shrink-0">
                    <User className="h-4 w-4 text-white" />
                  </div>
                )}
              </div>
            ))}
            {loading && (
              <div className="flex gap-2">
                <div className="h-7 w-7 rounded-full bg-emerald-500/20 flex items-center justify-center">
                  <Bot className="h-4 w-4 text-emerald-300" />
                </div>
                <div className="bg-white/10 rounded-2xl px-4 py-2.5 flex items-center gap-2 text-slate-300">
                  <Loader2 className="h-4 w-4 animate-spin" /> أشتغل على أدواتك…
                </div>
              </div>
            )}
          </div>

          <div className="p-4 border-t border-white/10 bg-white/5">
            <div className="max-w-3xl mx-auto w-full flex gap-2">
              <input
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && send()}
                placeholder="اكتب طلبك أو أمرك…"
                className="flex-1 bg-white/10 border border-white/15 rounded-xl px-4 py-3 outline-none focus:border-emerald-400 text-sm"
              />
              <button
                onClick={send}
                disabled={loading || !input.trim()}
                className="bg-emerald-600 hover:bg-emerald-500 disabled:opacity-40 rounded-xl px-4 text-white"
              >
                <Send className="h-4 w-4" />
              </button>
            </div>
          </div>
        </main>
      </div>

      {/* نافذة رمز الدخول */}
      {showAccess && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center p-4 z-50">
          <div className="bg-slate-900 border border-white/10 rounded-2xl p-6 w-full max-w-sm">
            <h3 className="font-bold text-lg mb-1">رمز الدخول</h3>
            <p className="text-xs text-slate-400 mb-4">
              {needAccess
                ? "الوكيل محمي برمز (AGENT_ACCESS_CODE). أدخله لتُكمل."
                : "الوكيل أفعاله تتطلب رمزاً إن كنت عيّنته على Vercel."}
            </p>
            <input
              autoFocus
              type="password"
              value={accessCode}
              onChange={(e) => setAccessCode(e.target.value)}
              placeholder="أدخل رمز الدخول"
              className="w-full bg-white/10 border border-white/15 rounded-xl px-3 py-2.5 mb-4 outline-none focus:border-emerald-400"
            />
            <div className="flex gap-2">
              <button
                onClick={() => {
                  setShowAccess(false);
                  setNeedAccess(false);
                }}
                className="flex-1 bg-white/10 rounded-xl py-2 text-sm"
              >
                إغلاق
              </button>
              <button
                onClick={() => {
                  setShowAccess(false);
                  setNeedAccess(false);
                }}
                className="flex-1 bg-emerald-600 rounded-xl py-2 text-sm"
              >
                حفظ
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
