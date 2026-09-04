// ---------- نماذج البيانات (تُطابق جداول Supabase) ----------
// عندك كل جدول هنا بنفس الأسماء اللي في schema.sql

export type ClientStatus = "new" | "active" | "paused" | "closed";

export interface Client {
  id: string;
  name: string;
  email?: string;
  phone?: string;
  whatsapp?: string;
  channel?: string; // منين جاء (إحالة / منصة / واتساب...)
  services?: string; // ما يشتريه
  hourly_rate?: number; // أجر الساعة (اختياري)
  budget?: number; // ميزانية متوقعة
  status: ClientStatus;
  next_follow_up?: string; // موعد المتابعة التالي (ISO)
  notes?: string;
  created_at: string;
}

export type ProjectStatus =
  | "lead"
  | "proposal"
  | "active"
  | "delivering"
  | "done"
  | "cancelled";

export interface Project {
  id: string;
  client_id?: string;
  name: string;
  description?: string;
  status: ProjectStatus;
  value?: number; // قيمة العقد
  currency?: string;
  deadline?: string;
  deliverables?: string;
  created_at: string;
}

export type InvoiceStatus = "draft" | "sent" | "paid" | "overdue";

export interface Invoice {
  id: string;
  number: string; // رقم الفاتورة، مثل 2026-0001
  client_id?: string;
  project_id?: string;
  amount: number;
  currency: string;
  status: InvoiceStatus;
  issue_date: string;
  due_date: string;
  notes?: string;
  created_at: string;
}

export type TaskStatus = "todo" | "doing" | "done";

export interface Task {
  id: string;
  title: string;
  description?: string;
  status: TaskStatus;
  project_id?: string;
  due_at?: string;
  created_at: string;
}

export interface Expense {
  id: string;
  label: string;
  amount: number;
  currency: string;
  category?: string; // أداة / اشتراك / تنقل / إعلانات...
  date: string;
  created_at: string;
}

export type ActionKind =
  | "send_invoice"
  | "send_message"
  | "schedule_followup"
  | "record_payment";

export type ActionStatus = "pending" | "approved" | "denied";

// حركة تتطلب موافقتك قبل التنفيذ (الأمان)
export interface PendingAction {
  id: string;
  kind: ActionKind;
  label: string; // وصف قصير ليطابق عليك
  payload: Record<string, unknown>;
  status: ActionStatus;
  created_at: string;
}
