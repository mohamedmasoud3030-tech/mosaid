import type { SupabaseClient } from "@supabase/supabase-js";
import { buildTools, type AgentTools } from "./tools";

/**
 * نظام مهارات، بنمط skills/ في Mosaid.
 * كل مهارة = منَعرّف (manifest) + مجموعة أدوات.
 * لإضافة مهارة: أنشئ module يصف tools خاصتك، ونسجّله هنا.
 * بهذا يكون الوكيل قابلاً للتوسّع دون تعديل النواة.
 */
export type SkillManifest = {
  id: string;
  name: string;
  description: string;
  /** هل قدراتها "خارجية" (تحتاج موافقة)؟ تُستخدم في وضعها في prompt و تقييدها. */
  externalAction: boolean;
};

export interface Skill {
  manifest: SkillManifest;
  /** اسم الأدوات التي توفرها (تسجيل تلقائي في سجل الجدول). */
  tools: string[];
}

/**
 * قائمة المهارات المفعّلة. كل عنصر يقابل مهارة في مشروع كود اليوم.
 * نوسّع بالحاجة (أدوات أكثر).
 */
export function getSkills(): Skill[] {
  return [
    {
      manifest: {
        id: "clients",
        name: "إدارة العملاء",
        description: "إنشاء/تحديث/البحث عن العملاء وحالتهم ومواعيد المتابعة.",
        externalAction: false,
      },
      tools: ["create_client", "update_client", "get_client", "get_clients", "set_follow_up"],
    },
    {
      manifest: {
        id: "projects",
        name: "المشاريع",
        description: "تحويل العملاء المحتملين إلى مشاريع، وتتبع حالاتها وقيمتها ومواعيدها.",
        externalAction: false,
      },
      tools: ["get_projects", "create_project", "update_project"],
    },
    {
      manifest: {
        id: "tasks",
        name: "المهام",
        description: "قائمة أولويات و تتبع المهام اليومية.",
        externalAction: false,
      },
      tools: ["get_tasks", "add_task", "update_task"],
    },
    {
      manifest: {
        id: "money",
        name: "الفلوس",
        description: "فواتير (مسودة فقط دون إرسال) + مصاريف + مؤشرات + تذكير بالمتأخرات.",
        externalAction: true, // إرسال/تسجيل دفع => موافقة
      },
      tools: ["get_invoices", "create_invoice", "record_payment", "get_expenses", "add_expense", "get_metrics"],
    },
    {
      manifest: {
        id: "memory",
        name: "الذاكرة",
        description: "حفظ/البحث/نسيان من ذاكرتك؛ أي نص يشبه سر يُرفض.",
        externalAction: false,
      },
      tools: ["remember", "search_memory", "forget_memory", "list_memory"],
    },
    {
      manifest: {
        id: "coding",
        name: "الترميز",
        description: "استيعاب مهام برمجية، طرح خطة/تسجيل — دون تنفيذ على نظام المستخدم.",
        externalAction: true,
      },
      tools: ["coding_plan"],
    },
  ];
}

/** بناء كل أدوات الوكيل، متضمنة محفظة مهاراتك، فوق اتصال Supabase. */
export function buildAgentTools(db: SupabaseClient): AgentTools {
  return buildTools(db);
}
