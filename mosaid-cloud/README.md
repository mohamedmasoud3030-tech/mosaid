# Mosaid Cloud — وكيلك الشخصي 💼

نسخة سحابية (Vercel + Supabase) **من فكرة Mosaid** — وكيل شخصي يعمل كأدوات *حقيقية*
لا مجرد محادثة. يعيش في بياناتك (عملاء، مشاريع، فواتير، مهام، مصاريف، ذاكرة) وينفّذ
عبر أدوات — وأي فعل خارجي (إرسال فاتورة، دفع، إرسال رسالة) يبقى **في انتظار موافقتك**،
مع **موافقة فردية** و**سجل تدقيق** و**fail-closed**.

> **مجاني بالكامل** · **مستضاف على Vercel** · **قاعدة بيانات Supabase** · **الموديل: Gemini (مجاني)**.
> التوافق المعماري مع `mosaid` موثّق في [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

---

## ماذا يستطيع؟
- **إدارة العملاء:** بحث، إضافة، تحديث، حالة، موعد متابعة.
- **المشاريع والمهام:** إنشاء وتحديث وربط بالعملاء.
- **الفواتير:** إنشاء مسودة + متابعة المدفوع والمتأخر، مع طلب موافقتك قبل "الإرسال".
- **المصاريف والأرباح:** تتبّع مصاريف، إجمالي مستحق، صافي، عملاء نشطون (لوحة نبذة جانبية).
- **الذاكرة:** `remember`/`search`/`forget` عبر FTS، مع رفض أي نص يشبه سراً، ومرشّحة → موافقة.
- **المهارات المنفصلة:** عملاء/مشاريع/مهام/فلوس/ذاكرة/ترميز — قابل للتوسع.
- **الحركات الخارجية:** تُخزَّن في `approval_requests` (موافقة فردية + TTL + استخدام مرة واحدة)
  وتظهر لك "في انتظار موافقتك". **التدقيق** في `audit_log`.

---

## هندسة النظام
```
(جوال/كمبيوتر)  ←→  Vercel (Next.js)  ←→  Supabase (Postgres)
                              │
                              └─ Agent Gemini loop (أدوات عبر Generated tools)
```
- **Next.js 15** على Vercel (مجاني، حافة قريبة).
- **Supabase** قاعدة بيانات المؤسسة + مفاتيح وإعدادات.
- **ai/AI SDK** لتشغيل أدوات الموديل.
- **أمان:** حارس `AGENT_ACCESS_CODE` + `pending_actions` كبقعة "كسر" قبل أي فعل.

---

## التشغيل خطوة بخطوة (من الصفر، بلا كود)

### 1. أنشئ حساب Supabase
- سجّل في [https://supabase.com](https://supabase.com) — مجاني.
- أنشئ مشروعاً (أي منطقة قريبة منك).
- من **SQL Editor** الصق محتوى `supabase/schema.sql` ثم نفّذ (انقر Run).
- انسخ من **Project Settings → API** قيمتي:
  - `Project URL` → **NEXT_PUBLIC_SUPABASE_URL**
  - `anon public` → **NEXT_PUBLIC_SUPABASE_ANON_KEY**

### 2. احصل على مفتاح الموديل (مجاني)
- انزل إلى [https://aistudio.google.com/apikey](https://aistudio.google.com/apikey) وأنشئ مفتاحاً → **GOOGLE_GENERATIVE_AI_API_KEY**.
- (أو استخدم OpenAI/Anthropic فيمكنك اختيارها بضبط `MODEL_PROVIDER` + المفتاح في الخطوة 3.)

### 3. انسخ الكود إلى GitHub
```bash
git init
git add .
git commit -m "رحلتي — وكيل الفريلانس"
git branch -M main
git remote add origin https://github.com/حسابك/rihlati-agent.git
git push -u origin main
```
(أو أنشئ مستودعاً جديداً وارفع الملفات من الواجهة — أسهل لغير المبرمج.)

### 4. الانشر على Vercel [مجاني]
- سجّل في [https://vercel.com](https://vercel.com) (ادخل بحساب GitHub).
- **Add New → Project → Import** مستودعك، واختر Next.js تلقائياً.
- أضف هذه البيئات في **Environment Variables** (تماماً كما في `.env.example`):
  ```
  NEXT_PUBLIC_SUPABASE_URL      = https://xxxx.supabase.co
  NEXT_PUBLIC_SUPABASE_ANON_KEY = eyJhbGci...
  GOOGLE_GENERATIVE_AI_API_KEY  = AIza...
  AGENT_ACCESS_CODE             = سرّك
  ```
- انقر **Deploy**. بعد دقيقة تفتح Link (مثل `rihlati-agent.vercel.app`).
- افتح الموقع وأدخل `AGENT_ACCESS_CODE` (زر "رمز الدخول").

---

## التطوير محلياً
```bash
npm install
cp .env.example .env.local   # ثم عبّئ القيم
npm run dev                  # localhost:3000
```

## توسّع (مهارات جديدة)
كل "أداة" تُعرَّف في `lib/agent/tools.ts`. لإضافة قدرة (مثل إرسال إيميل فعلي أو WhatsApp):
1. أضِف دالة داخل `buildTools`.
2. صِف ماذا تفعل في `description`.
3. (اختياري) صل خدمة خارجية/API — واستمر في الاعتماد على `require_approval`.

لا تُشرك إيميل فعلياً قبل فهم المخاطر؛ حاليّاً `send_invoice`/`send_message` **مُحاكاة** (تُسجَّل حالة الفاتورة وطلب الموافقة) وتُنفَّذ عند موافقتك — وعليك ربط مزوّد رسائل حقيقي لاحقاً.

---

## الخصوصية و الحدود
- البيانات تبقى بينك وبين Supabase؛ الموديل يُستدعى بمفتاحك (Gemini).
- **لا تضع أي بيانات حساسة** (كلمات سر، بيانات بنكية كاملة) في أدوات الوكيل؛ أبقِ الوصول الجزئي.
- كل فعل له أثر خارجي يُرتجى موافقتك، وحارس `AGENT_ACCESS_CODE` يَحجب واجهة الـ API.
- لتصعد الأمان لاحقاً: فعّل **Row Level Security** في Supabase وأنشئ سياسات تخص `authenticated`.

---

## الملفات
```
app/            واجهة الويب + محوّلات الـ API (agent, approvals, metrics)
lib/            نصوص، أدوات، مزوّد الموديل، Supabase، أمان
supabase/       مخطط قاعدة البيانات
.env.example    القيم المطلوبة
PLAN.md         خطة الجلسة (الخريطة الكاملة)
```
