import { createHash } from "crypto";
import { tool } from "ai";
import { z } from "zod";
import type { SupabaseClient } from "@supabase/supabase-js";
import { Audit } from "@/lib/core/audit";
import { createApproval } from "@/lib/core/approval";
import { redact } from "@/lib/core/redact";
import {
  addCandidate,
  approveCandidate,
  remember,
  search as memorySearch,
  forget as memoryForget,
  offerCandidate,
} from "@/lib/core/memory";

type Db = SupabaseClient;
const table = (db: Db, n: string) => db.from(n);
const argsHash = (a: unknown) => createHash("sha256").update(JSON.stringify(a)).digest("hex").slice(0, 16);

export function buildTools(db: Db) {
  return {
    // =========================================================
    //  العملاء
    // =========================================================
    get_clients: tool({
      description: "بحث/قائمة العملاء حسب الاسم أو الحالة.",
      parameters: z.object({
        q: z.string().optional(),
        status: z.string().optional(),
        limit: z.number().optional().default(50),
      }),
      execute: async ({ q, status, limit }) => {
        let query = table(db, "clients").select("*").order("created_at", { ascending: false }).limit(limit ?? 50);
        if (status) query = query.eq("status", status);
        if (q) query = query.or(`name.ilike.%${q}%,email.ilike.%${q}%`);
        const { data, error } = await query;
        return error ? { error: error.message } : data;
      },
    }),

    get_client: tool({
      description: "تفاصيل عميل بالمعرّف.",
      parameters: z.object({ id: z.string() }),
      execute: async ({ id }) => {
        const { data, error } = await table(db, "clients").select("*").eq("id", id).single();
        return error ? { error: error.message } : data;
      },
    }),

    create_client: tool({
      description: "إضافة عميل جديد (آمن، بلا موافقة).",
      parameters: z.object({
        name: z.string(),
        email: z.string().optional(),
        phone: z.string().optional(),
        whatsapp: z.string().optional(),
        channel: z.string().optional(),
        services: z.string().optional(),
        budget: z.number().optional(),
        status: z.string().optional().default("new"),
        notes: z.string().optional(),
      }),
      execute: async (args) => {
        const { data, error } = await table(db, "clients")
          .insert({ ...args, status: args.status || "new" })
          .select()
          .single();
        if (!error) await Audit(db, { kind: "client_created", resource: (data as any)?.id, detail: { name: args.name } });
        return error ? { error: error.message } : data;
      },
    }),

    update_client: tool({
      description: "تحديث عميل (حالة، موعد متابعة، ملاحظات...).",
      parameters: z.object({ id: z.string(), fields: z.record(z.string(), z.any()) }),
      execute: async ({ id, fields }) => {
        const { data, error } = await table(db, "clients").update(fields).eq("id", id).select().single();
        if (!error) await Audit(db, { kind: "client_updated", resource: id });
        return error ? { error: error.message } : data;
      },
    }),

    set_follow_up: tool({
      description: "ضبط موعد متابعة عميل (يُستخدم للتنبيه غير الصادر).",
      parameters: z.object({ id: z.string(), date: z.string() }),
      execute: async ({ id, date }) => {
        const { data, error } = await table(db, "clients")
          .update({ next_follow_up: date })
          .eq("id", id)
          .select()
          .single();
        if (!error) await Audit(db, { kind: "followup_set", resource: id, detail: { date } });
        return error ? { error: error.message } : data;
      },
    }),

    // =========================================================
    //  المشاريع
    // =========================================================
    get_projects: tool({
      description: "المشاريع حسب الحالة (lead/proposal/active/delivering/done/cancelled).",
      parameters: z.object({ status: z.string().optional(), limit: z.number().optional().default(50) }),
      execute: async ({ status, limit }) => {
        let query = table(db, "projects").select("*").order("created_at", { ascending: false }).limit(limit ?? 50);
        if (status) query = query.eq("status", status);
        const { data, error } = await query;
        return error ? { error: error.message } : data;
      },
    }),

    create_project: tool({
      description: "إضافة مشروع (آمن).",
      parameters: z.object({
        name: z.string(),
        client_id: z.string().optional(),
        description: z.string().optional(),
        status: z.string().optional().default("lead"),
        value: z.number().optional(),
        deadline: z.string().optional(),
        deliverables: z.string().optional(),
      }),
      execute: async (args) => {
        const { data, error } = await table(db, "projects")
          .insert({ ...args, status: args.status || "lead" })
          .select()
          .single();
        return error ? { error: error.message } : data;
      },
    }),

    update_project: tool({
      description: "تحديث مشروع (حالة، قيمة، موعد).",
      parameters: z.object({ id: z.string(), fields: z.record(z.string(), z.any()) }),
      execute: async ({ id, fields }) => {
        const { data, error } = await table(db, "projects").update(fields).eq("id", id).select().single();
        return error ? { error: error.message } : data;
      },
    }),

    // =========================================================
    //  المهام
    // =========================================================
    get_tasks: tool({
      description: "المهام حسب الحالة (todo/doing/done).",
      parameters: z.object({ status: z.string().optional(), limit: z.number().optional().default(50) }),
      execute: async ({ status, limit }) => {
        let query = table(db, "tasks").select("*").order("created_at", { ascending: false }).limit(limit ?? 50);
        if (status) query = query.eq("status", status);
        const { data, error } = await query;
        return error ? { error: error.message } : data;
      },
    }),

    add_task: tool({
      description: "إضافة مهمة.",
      parameters: z.object({
        title: z.string(),
        description: z.string().optional(),
        project_id: z.string().optional(),
        due_at: z.string().optional(),
      }),
      execute: async (args) => {
        const { data, error } = await table(db, "tasks").insert({ ...args, status: "todo" }).select().single();
        return error ? { error: error.message } : data;
      },
    }),

    update_task: tool({
      description: "تحديث مهمة (تم/قيد التنفيذ).",
      parameters: z.object({ id: z.string(), fields: z.record(z.string(), z.any()) }),
      execute: async ({ id, fields }) => {
        const { data, error } = await table(db, "tasks").update(fields).eq("id", id).select().single();
        return error ? { error: error.message } : data;
      },
    }),

    // =========================================================
    //  الفلوس (مؤشرات + فواتير مسودة — الإرسال يتطلب موافقة)
    // =========================================================
    get_invoices: tool({
      description: "الفواتير حسب الحالة (draft/sent/paid/overdue).",
      parameters: z.object({ status: z.string().optional(), limit: z.number().optional().default(50) }),
      execute: async ({ status, limit }) => {
        let query = table(db, "invoices").select("*").order("created_at", { ascending: false }).limit(limit ?? 50);
        if (status) query = query.eq("status", status);
        const { data, error } = await query;
        return error ? { error: error.message } : data;
      },
    }),

    create_invoice: tool({
      description:
        "إنشاء فاتورة في مسودة (draft) — لا يُرسل أبداً تلقائياً. الإرسال يتطلب موافقة.",
      parameters: z.object({
        number: z.string().optional(),
        client_id: z.string().optional(),
        project_id: z.string().optional(),
        amount: z.number(),
        currency: z.string().optional().default("USD"),
        due_date: z.string().optional(),
        notes: z.string().optional(),
      }),
      execute: async (args) => {
        const number = args.number || `0000-${Date.now()}`;
        const { data, error } = await table(db, "invoices")
          .insert({
            number,
            client_id: args.client_id,
            project_id: args.project_id,
            amount: args.amount,
            currency: args.currency || "USD",
            status: "draft",
            issue_date: new Date().toISOString().slice(0, 10),
            due_date: args.due_date || new Date(Date.now() + 30 * 864e5).toISOString().slice(0, 10),
            notes: args.notes,
          })
          .select()
          .single();
        if (!error) await Audit(db, { kind: "invoice_drafted", resource: (data as any)?.id, detail: { amount: args.amount } });
        return error ? { error: error.message } : data;
      },
    }),

    get_expenses: tool({
      description: "قائمة المصاريف.",
      parameters: z.object({ limit: z.number().optional().default(50) }),
      execute: async ({ limit }) => {
        const { data, error } = await table(db, "expenses").select("*").order("created_at", { ascending: false }).limit(limit ?? 50);
        return error ? { error: error.message } : data;
      },
    }),

    add_expense: tool({
      description: "إضافة مصروف (آمن).",
      parameters: z.object({
        label: z.string(),
        amount: z.number(),
        currency: z.string().optional().default("USD"),
        category: z.string().optional(),
        date: z.string().optional(),
      }),
      execute: async (args) => {
        const { data, error } = await table(db, "expenses")
          .insert({ ...args, date: args.date || new Date().toISOString().slice(0, 10) })
          .select()
          .single();
        return error ? { error: error.message } : data;
      },
    }),

    get_metrics: tool({
      description: "ملخّص مالي: مستحق/مدفوع/متأخر/مصاريف/صافي/عملاء نشطون.",
      parameters: z.object({}),
      execute: async () => {
        const [{ data: invoices }, { data: expenses }, { data: clients }] = await Promise.all([
          table(db, "invoices").select("*"),
          table(db, "expenses").select("*"),
          table(db, "clients").select("id,status"),
        ]);
        const list = invoices ?? [];
        const unpaid = list.filter((i) => i.status === "sent").reduce((a, i) => a + (i.amount || 0), 0);
        const paid = list.filter((i) => i.status === "paid").reduce((a, i) => a + (i.amount || 0), 0);
        const draft = list.filter((i) => i.status === "draft").reduce((a, i) => a + (i.amount || 0), 0);
        const overdue = list
          .filter((i) => i.status === "sent" && i.due_date && i.due_date < new Date().toISOString())
          .reduce((a, i) => a + (i.amount || 0), 0);
        const exp = (expenses ?? []).reduce((a, e) => a + (e.amount || 0), 0);
        return {
          unpaid, paid, draft, overdue, expenses: exp,
          active_clients: (clients ?? []).filter((c) => c.status === "active").length,
          net: paid - exp,
        };
      },
    }),

    // =========================================================
    //  الذاكرة (آمنة: رفض أسرار، مرشّحة ثم موافقة أو فورية)
    // =========================================================
    remember: tool({
      description: "حفظ نص في الذاكرة (فوري). يُرفض أي نص يشبه سراً.",
      parameters: z.object({ content: z.string() }),
      execute: async ({ content }) => {
        try {
          const r = await remember(db, content);
          await Audit(db, { kind: "memory_saved", resource: r.id });
          return { ok: true, id: r.id };
        } catch (e) {
          return { ok: false, error: (e as Error).message };
        }
      },
    }),

    suggest_memory: tool({
      description: "عرض نص للذاكرة كمرشّحة (لا تُحفظ إلا بموافقة لاحقة).",
      parameters: z.object({ content: z.string(), confidence: z.number().optional().default(0.5) }),
      execute: async ({ content, confidence }) => {
        try {
          const c = await addCandidate(db, content, "agent", confidence);
          await Audit(db, { kind: "memory_candidate", resource: c.id });
          return { ok: true, candidate_id: c.id, pending: "await approval" };
        } catch (e) {
          return { ok: false, error: (e as Error).message };
        }
      },
    }),

    approve_memory_candidate: tool({
      description: "موافقة على مرشّحة ذاكرة (تصبح عنصراً فعلياً).",
      parameters: z.object({ candidate_id: z.string() }),
      execute: async ({ candidate_id }) => {
        try {
          const r = await approveCandidate(db, candidate_id);
          return { ok: true, id: r.id };
        } catch (e) {
          return { ok: false, error: (e as Error).message };
        }
      },
    }),

    search_memory: tool({
      description: "البحث في الذاكرة.",
      parameters: z.object({ query: z.string(), limit: z.number().optional().default(20) }),
      execute: async ({ query, limit }) => {
        const r = await memorySearch(db, query, limit);
        return redact(JSON.stringify(r ?? []));
      },
    }),

    list_memory: tool({
      description: "قائمة بالذاكرة المحفوظة (آخر 50).",
      parameters: z.object({}),
      execute: async () => {
        const { data, error } = await table(db, "memory_items").select("id,content,source,confidence,created_at").order("created_at", { ascending: false }).limit(50);
        return error ? { error: error.message } : data;
      },
    }),

    forget_memory: tool({
      description: "حذف عنصر من الذاكرة.",
      parameters: z.object({ id: z.string() }),
      execute: async ({ id }) => {
        await memoryForget(db, id);
        return { ok: true };
      },
    }),

    // =========================================================
    //  طلبات الموافقة للأفعال الخارجية (موافقة فردية مرتبطة)
    // =========================================================
    submit_approval: tool({
      description:
        "ارفع طلب موافقة على فعل خارجي (إرسال فاتورة/رسالة، تذكير، تسجيل دفع). لا تنفّذ الفعل بنفسك أبداً — فقط ارفع هذا الطلب.",
      parameters: z.object({
        kind: z.enum([
          "send_invoice",
          "send_message",
          "schedule_followup",
          "record_payment",
          "run_code",
        ]),
        label: z.string().describe("وصف قصير واضح"),
        resource: z.string().describe("المورد: مثلا رقم الفاتورة او بريد العميل"),
        payload: z.record(z.string(), z.any()).describe("التفاصيل (مبلغ، بريد، رقم فاتورة...)"),
      }),
      execute: async ({ kind, label, resource, payload }) => {
        const req = await createApproval(db, {
          tool: kind,
          argsHash: argsHash(payload),
          resource,
        });
        await Audit(db, { kind: "external_action_requested", resource, decision: "pending", detail: { kind, label } });
        return {
          ok: true,
          message: `رفعت طلب موافقة: ${label}`,
          approval_id: req.id,
          expires: req.expires,
        };
      },
    }),

    list_approvals: tool({
      description: "عرض الحركات في انتظار موافقة المالك.",
      parameters: z.object({}),
      execute: async () => {
        const { data, error } = await table(db, "approval_requests")
          .select("*")
          .eq("state", "pending")
          .order("created_at", { ascending: false });
        return error ? { error: error.message } : data;
      },
    }),

    // =========================================================
    //  مهارة الترميز (خطط فقط — دون تنفيذ)
    // =========================================================
    coding_plan: tool({
      description: "طرح خطة هندسية/رمزية كخطوات — لا تنفيذ، لا وصول لنظام المستخدم.",
      parameters: z.object({
        prompt: z.string().describe("ما الذي يريد المالك إنجازه تقنياً؟"),
      }),
      execute: async ({ prompt }) => {
        await Audit(db, { kind: "coding_plan_requested", detail: { prompt: redact(prompt) } });
        return {
          ok: true,
          note: "خطة فقط. لا ننفّذ ولا نصل إلى نظام المالك. أخبره بأنه يمكن تقييمها في المحادثة.",
        };
      },
    }),
  };
}

export type AgentTools = ReturnType<typeof buildTools>;
