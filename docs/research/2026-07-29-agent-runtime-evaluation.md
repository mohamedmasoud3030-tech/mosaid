# تقييم أساس وكيل Mosaid العام — 2026-07-29

## حالة الوثيقة

- النوع: تقرير بحث ومقارنة ومعمارية قبل التنفيذ.
- تاريخ القطع البحثي: 29 يوليو 2026، Asia/Muscat.
- الهدف: اختيار أبسط أساس قابل للصيانة لوكيل شخصي عام يعمل مبدئيًا على Android/Termux وتتم إدارته من Telegram.
- القرار الحالي: **اختيار PicoClaw v0.3.1 بصورة مشروطة لإجراء Phase 0 فقط، وليس اعتمادًا نهائيًا للمنتج.**
- مستوى الثقة وقت القرار: 78% إجمالًا؛ 84% في الاتجاه المعماري، و62% فقط في موثوقية Android قبل الاختبار الفعلي.

## 1. المتطلبات التي حددت القرار

المطلوب ليس منصة وكلاء ضخمة ولا وكيلًا مخصصًا لمشروع واحد، بل نواة شخصية صغيرة تبدأ بالبرمجة والمستودعات، ثم تتوسع إلى:

- الملفات والاختبارات وGit/GitHub.
- البحث وتحليل المصادر والمستندات.
- الذاكرة والجدولة والتنبيهات.
- Telegram long polling كواجهة أساسية.
- Skills وMCP لاحقًا.
- توليد الصور وخدمات النشر الرسمية مثل Meta Graph API لاحقًا.

القيود الأعلى وزنًا:

1. Android/Termux وARM64 فعليان، لا مجرد عبارة “Linux supported”.
2. استهلاك منخفض للذاكرة والطاقة وسرعة بدء مناسبة لهاتف قديم.
3. نموذج أمان يمكن جعله deny-by-default، مع owner allowlist وموافقات للعمليات الحساسة.
4. مشروع مفهوم يمكن تقليصه، لا منصة متعددة الخدمات لا يمكن صيانتها.
5. ترخيص يسمح بالاشتقاق وإعادة الاستخدام.
6. عدم الاعتماد على Docker، لأن Docker والعزل القائم عليه غير متاحين طبيعيًا داخل Termux.

## 2. الحكم التنفيذي

### الاتجاه المختار

الاختيار كان **Fork أمني مصغّر من PicoClaw مثبت على v0.3.1، بعد نجاح Phase 0**، مع استخدام أجزاء محددة فقط:

- Agent loop وMessage bus.
- Telegram long polling.
- واجهات مزودي النماذج، خصوصًا OpenAI-compatible.
- نظام الأدوات كأساس شكلي، لا بسياساته الأمنية الحالية.
- MCP client المبني على SDK الرسمي للغة Go في مرحلة لاحقة.
- الجلسات والحالة والجدولة كأساس أولي.
- مسار بناء Android/ARM64.
- الاختبارات الحالية حول Telegram وShell وMCP.

لا يعني القرار تشغيل PicoClaw كما هو. قبل أي منتج يجب حذف معظم المنصة وإعادة كتابة:

- Permission/Approval Engine.
- Shell execution.
- Workspace and symlink boundary.
- Audit log and secret boundary.
- Durable Telegram inbox/outbox and crash recovery.

### لماذا PicoClaw؟

- الإصدار v0.3.1 نشر Android ARM64 artifact رسميًا، وليس Linux ARM64 فقط: [release v0.3.1](https://github.com/sipeed/picoclaw/releases/tag/v0.3.1).
- Telegram موثق كـlong polling.
- المشروع MIT: [LICENSE](https://github.com/sipeed/picoclaw/blob/v0.3.1/LICENSE).
- Go ملائم لبيئة الهاتف: binary واحد ولا يحتاج Python virtualenv أو Node runtime دائمًا.
- يوجد CI فعلي يشمل lint والاختبارات و`govulncheck`: [PR workflow](https://github.com/sipeed/picoclaw/blob/v0.3.1/.github/workflows/pr.yml).
- يدعم MCP عبر `modelcontextprotocol/go-sdk`، ويوجد فصل معقول بين agent/bus/channels/providers/tools.

### لماذا لا يُستخدم كما هو؟

`pkg/tools/shell.go` في v0.3.1 يعتمد إلى حد كبير على regex denylist، ويمكن تعطيل أنماط المنع، كما أن defaults في ذلك الإصدار تشغل أدوات كثيرة و`allow_remote` كان واسعًا. كذلك كان `allow_from` الفارغ يعني السماح للجميع. هذه خواص غير مقبولة لمنتج شخصي يملك وصولًا إلى مستودعات وأسرار.

الدليل الكودي: [pkg/tools/shell.go](https://github.com/sipeed/picoclaw/blob/v0.3.1/pkg/tools/shell.go)، و[config defaults](https://github.com/sipeed/picoclaw/blob/v0.3.1/pkg/config/defaults.go).

## 3. جدول المقارنة

الأوزان المستخدمة: Android/Termux 25%، الخفة 20%، الأمان 20%، Telegram/Tools/MCP/Memory/Cron 15%، الصيانة والاختبارات 10%، الترخيص 10%.

| المرشح | الترخيص والنشاط عند القطع | Android/Termux | Telegram/Tools/MCP/Memory | العيوب الحاسمة | الملاءمة |
|---|---|---|---|---|---:|
| **PicoClaw v0.3.1** | MIT؛ إصدار 2026-07-03؛ CI واختبارات | Android artifact رسمي وTermux موثق | Long polling، MCP، Skills، Cron، Memory، Files/Shell | المصدر وgo.mod أكبر من التسويق؛ Shell غير مقبول أمنيًا؛ يحتاج pruning | **4.3/5** |
| **Hermes Agent v0.19.0** | MIT؛ إصدار 2026-07-20؛ نشاط واختبارات ضخمة | دعم رسمي Tier 2؛ الخلفية best-effort؛ لا Docker | الأكثر اكتمالًا: gateway، approvals، skills، memory، cron، providers | منصة كبيرة جدًا وسريعة التغير؛ سطح هجوم واسع | 3.8/5 |
| **nanobot v0.3.0** | MIT؛ إصدار 2026-07-25؛ CI وcoverage 75% | نظري؛ لا مسار Android رسمي مماثل | Telegram، MCP، memory، cron، image، documents | Alpha؛ Pydantic/tiktoken/lxml وحزم مستندات؛ تضخم ملحوظ | 3.7/5 |
| **PydanticAI v2.20.0** | MIT؛ فريق واختبارات قوية | غير مدعوم رسميًا؛ مخاطر pydantic-core في Termux | Typed tools، MCP، HITL، providers، durable integrations | ليس personal runtime؛ يلزم بناء Telegram/Memory/Scheduler/Recovery | 3.1/5 |
| **OpenClaw 2026.7.x** | MIT؛ مؤسسة ونشاط قوي | تشغيل مجتمعي؛ خدمة daemon على Termux غير مستقرة | أقوى channels/skills/MCP/approvals/sandbox | منصة ضخمة جدًا وNode/plugin ecosystem واسع | 2.9/5 |
| **LangGraph 1.2.10** | MIT؛ إنتاجي ونشط | Python/Pydantic؛ دعم Android نظري | Durable checkpoints، HITL، memory | منخفض المستوى؛ يحتاج بناء runtime كامل | 2.6/5 |
| **Open Interpreter Rust 0.0.34** | Apache-2.0؛ نشط | لا Android artifact واضح | Coding agent، MCP، Skills، approvals | ليس personal runtime؛ لا Telegram/Cron/long-term personal memory | 2.3/5 |
| **CrewAI 1.15.8** | MIT؛ نشط | نظري فقط | Crews/Flows/Memory/HITL/MCP | يحل orchestration لفرق وكلاء لا runtime شخصيًا صغيرًا | 2.1/5 |
| **OpenHands 1.6.1** | MIT؛ مشروع قوي | Docker هو العزل الموصى به؛ غير مناسب للهاتف | برمجة ومستودعات ممتازة | Node/Python/Docker/Web UI، ثقيل جدًا، لا Telegram-first | 1.7/5 |
| **Agent Zero 2.7** | LICENSE الحالي MIT؛ نشط | Docker والاعتمادات الثقيلة تمنع الملاءمة | Linux desktop، browser، documents، plugins، memory | FAISS/Playwright/Whisper/unstructured؛ أكبر من الحاجة | 1.4/5 |
| **AutoGen 0.7.5** | MIT للكود؛ maintenance mode رسميًا | نظري | Multi-agent وMCP | غير مناسب لمشروع جديد؛ Microsoft يوجه إلى Agent Framework | 1.3/5 |

## 4. الأدلة التفصيلية

### 4.1 PicoClaw

- المصدر: [sipeed/picoclaw](https://github.com/sipeed/picoclaw).
- الإصدار المثبت بحثيًا: [v0.3.1](https://github.com/sipeed/picoclaw/releases/tag/v0.3.1).
- الترخيص: MIT.
- Go directive في الإصدار: `go 1.25.11`: [go.mod](https://github.com/sipeed/picoclaw/blob/v0.3.1/go.mod).
- Release workflow يستخدم tag checkout ثم setup-go من go.mod ثم GoReleaser: [release.yml](https://github.com/sipeed/picoclaw/blob/v0.3.1/.github/workflows/release.yml).
- Android bundle ينتج من `GOOS=android GOARCH=arm64` ويحتوي core وlauncher: [Makefile](https://github.com/sipeed/picoclaw/blob/v0.3.1/Makefile).
- MCP يستخدم SDK الرسمي للغة Go.
- Telegram adapter والاختبارات موجودة تحت `pkg/channels/telegram/`.
- Shell الحالي يستخدم نص shell وregex patterns؛ هو accident-prevention وليس containment.
- `.security.yml` يفصل الأسرار تنظيميًا، لكنه يبقى plaintext محميًا بصلاحيات الملف، وليس key vault أو sandbox.

الاستنتاج: أفضل نقطة بدء تشغيلية للهاتف، لكنه **غير صالح أمنيًا كما هو**.

### 4.2 Hermes Agent

- المستودع: [NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent).
- الترخيص: MIT.
- الإصدار عند القطع: v0.19.0 / 2026.7.20.
- Python `>=3.11,<3.14` واعتمادات كثيرة مثبتة: [pyproject.toml](https://github.com/NousResearch/hermes-agent/blob/main/pyproject.toml).
- Termux موثق رسميًا لكنه Tier 2، مع core/Telegram/MCP/Cron، ودون Docker، واستمرارية الخلفية best-effort: [Termux guide](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/getting-started/termux.md).
- Telegram long polling وallowlist/pairing موثقان رسميًا.
- سياسة الأمان صريحة بأن الحاجز الوحيد أمام LLM عدائي هو OS-level isolation، وأن approval/denylist ليست containment: [SECURITY.md](https://github.com/NousResearch/hermes-agent/blob/main/SECURITY.md).

الاستنتاج: أفضل مرجع أمني وتشغيلي، لكنه كبير جدًا، وميزة عزله الأساسية غير متاحة داخل Termux.

### 4.3 nanobot

- المستودع: [HKUDS/nanobot](https://github.com/HKUDS/nanobot).
- الترخيص: MIT.
- الإصدار: v0.3.0.
- Python `>=3.11`، وbase dependencies تشمل Pydantic وtiktoken وlxml وMCP وPDF/Word/Excel/PowerPoint: [pyproject.toml](https://github.com/HKUDS/nanobot/blob/main/pyproject.toml).
- Telegram long polling و`allowFrom` موثقان: [chat-apps.md](https://github.com/HKUDS/nanobot/blob/main/docs/chat-apps.md).
- CI على إصدارات Python متعددة وWindows، واختبارات WebUI/Docker وcoverage threshold: [.github/workflows/ci.yml](https://github.com/HKUDS/nanobot/blob/main/.github/workflows/ci.yml).
- workspace guards موثقة بأنها application-level وليست OS sandbox: [workspace_policy.py](https://github.com/HKUDS/nanobot/blob/main/nanobot/security/workspace_policy.py).

الاستنتاج: المنافس الأقرب وظيفيًا، لكن سلسلة Python الأصلية أصعب على Android والنواة الحالية لم تعد صغيرة بالمعنى العملي.

### 4.4 OpenClaw

- المستودع: [openclaw/openclaw](https://github.com/openclaw/openclaw).
- الترخيص: MIT مع `THIRD_PARTY_NOTICES.md`.
- Runtime يتطلب Node حديثًا، والمستودع واسع جدًا ويشمل apps/plugins/gateway/channels.
- نقاط قوة: pairing، owner allowlists، exec approvals مرتبطة بالوسيطات، sandbox policies، memory، cron، MCP filters، threat model مفصل.
- أمان المشروع يفترض trusted single operator، ولا يعد shared gateway multi-tenant boundary.
- توجد تقارير Android/Termux بأن gateway foreground يعمل لكن تثبيت الخدمة ليس مسارًا رسميًا مستقرًا.

الاستنتاج: مصدر ممتاز لأنماط approvals/threat modeling، لكنه النقيض من شرط الخفة وقابلية الصيانة على هاتف قديم.

### 4.5 PydanticAI

- المستودع: [pydantic/pydantic-ai](https://github.com/pydantic/pydantic-ai).
- الترخيص: MIT.
- الإصدار عند القطع: v2.20.0.
- يدعم providers كثيرة، typed tools، structured output، MCP، deferred tools، human approval، graphs، tracing/evals.
- MCP client يدعم stdio وStreamable HTTP وSSE: [docs/mcp/client.md](https://github.com/pydantic/pydantic-ai/blob/main/docs/mcp/client.md).
- approval/deferred tools مصممة جيدًا: [deferred-tools.md](https://github.com/pydantic/pydantic-ai/blob/main/docs/deferred-tools.md).
- durable execution الرسمي يتكامل مع Temporal وDBOS وPrefect وRestate: [durable execution](https://github.com/pydantic/pydantic-ai/blob/main/docs/durable_execution/overview.md).
- حتى slim يعتمد Pydantic core، وهو Rust extension يسبب احتكاكًا معروفًا على Termux.

الاستنتاج: أفضل إطار Python لو كان الهدف سيرفرًا عاديًا، لكنه لا يحل personal runtime ويضيف مخاطر Android packaging.

### 4.6 LangGraph

- المستودع: [langchain-ai/langgraph](https://github.com/langchain-ai/langgraph).
- الترخيص: MIT.
- الإصدار: 1.2.10، Python 3.10+، يعتمد LangChain Core وPydantic وcheckpoint packages.
- نقاط القوة: durable execution، interrupts/HITL، short/long memory، streaming، recovery.
- ليس Telegram runtime ولا tool/security layer جاهزة للهدف.

الاستنتاج: أقوى من الحاجة في orchestration، وأضعف من الحاجة في runtime الجاهز للهاتف.

### 4.7 Open Interpreter

- المستودع: [openinterpreter/openinterpreter](https://github.com/openinterpreter/openinterpreter).
- النسخة الحالية أعيدت في Rust ومبنية على Codex-style harness.
- الترخيص: Apache-2.0.
- يدعم MCP/ACP/skills/permissions/sandbox، لكنه coding agent terminal-first.
- المشروع الأصلي Python أصبح community-maintained fork منفصلًا.

الاستنتاج: مفيد كوكيل برمجة أو subprocess مستقبلي، وليس نواة وكيل شخصي دائم عبر Telegram.

### 4.8 OpenHands

- المستودع: [OpenHands/OpenHands](https://github.com/OpenHands/OpenHands).
- الترخيص: MIT للنواة المفتوحة.
- نقاط القوة: coding performance، real repositories، shell/browser، task automation، agent server/canvas.
- التشغيل الموصى به للأمان يعتمد Docker؛ التشغيل المباشر يعطي وصولًا كاملاً للمضيف.
- يعتمد Node/Python/Web UI وصورًا وحزمًا كبيرة.

الاستنتاج: مناسب لسيرفر تطوير أو VM، لا لهاتف Termux صغير.

### 4.9 Agent Zero

- المستودع: [agent0ai/agent-zero](https://github.com/agent0ai/agent-zero).
- LICENSE الحالي MIT.
- Dockerized Linux desktop، browser، documents، plugin hub، memory، multi-agent.
- dependencies تشمل FAISS وPlaywright وWhisper وunstructured وsentence-transformers وغيرها.

الاستنتاج: غني جدًا لكنه منصة desktop/container، لا runtime هاتف.

### 4.10 AutoGen

- المستودع: [microsoft/autogen](https://github.com/microsoft/autogen).
- `LICENSE-CODE` MIT.
- README يعلن maintenance mode وعدم إضافة features جديدة ويوجه المشاريع الجديدة إلى Microsoft Agent Framework.

الاستنتاج: غير مناسب لبدء مشروع جديد طويل العمر.

### 4.11 CrewAI

- المستودع: [crewAIInc/crewAI](https://github.com/crewAIInc/crewAI).
- الترخيص: MIT.
- Crews للفرق متعددة الوكلاء وFlows للأتمتة الحدثية، مع Memory/HITL/MCP.
- لا يوفر Telegram-first personal runtime أو سياسة جهاز محلي صغيرة.

الاستنتاج: abstraction في غير موضعها بالنسبة للهدف.

## 5. المشاريع الإضافية المفحوصة

### ZeroClaw

Rust، single binary، providers/channels/security traits كثيرة، لكنه أصبح workspace كبيرًا. ظهر فشل بناء فعلي على Termux في issue #880، ولم توجد مصفوفة Android CI كافية وقت البحث. لم يُرفع إلى الاختيار النهائي.

### NullClaw

Zig وruntime شديد الصغر مع providers/channels/tools/MCP، لكنه حديث ودليل Android/Termux الرسمي أقل نضجًا من PicoClaw.

### Moltis

Rust-native personal agent مع sandbox/MCP/memory/Telegram، لكنه مئات آلاف الأسطر وعشرات crates، أقرب إلى منصة كاملة.

### Picobot

Go binary صغير وTelegram/memory/skills/MCP، لكنه أقل نضجًا في الاختبارات والأمن والمجتمع من PicoClaw.

### Mercury Agent

Permission-first وTelegram جيدان، لكنه مشروع TypeScript حديث ودليل Android ضعيف.

### QwenPaw وOpenHarness

قدرات شخصية واسعة وWeb UI وPython، لكنهما منصتان كاملتان لا أساسًا مصغرًا للهاتف.

## 6. خيارات التصميم التي قورنت

### A — Fork مشروع واحد وتنظيفه

مناسب فقط إذا كان المشروع خفيف runtime وله Android evidence. PicoClaw هو الوحيد الذي اجتاز هذا الشرط بما يكفي لمرحلة qualification. الخطر هو صعوبة مزامنة upstream بعد إعادة كتابة الأمن.

### B — نواة من مشروع واستعارة أجزاء من أخرى

رفض كنهج تنفيذ أولي لتجنب تجميع هش. المسموح هو استعارة الأنماط الأمنية فقط، لا تركيب runtimes متعددة.

### C — إطار Agent صغير وكتابة runtime

PydanticAI كان المرشح الأقوى هنا، لكن Python/Termux وضرورة كتابة Telegram/Memory/Scheduler/Recovery جعلته أقل عملية.

### D — نواة جديدة بالكامل

أفضل سيطرة وأقل تراثًا، لكنه يعيد اختراع provider/tool loop/MCP/session details ويزيد زمن الوصول إلى PoC.

### القرار

A بصورة مضبوطة: qualification أولًا، ثم fork مصغر فقط عند نجاح الهاتف. إذا فشل Phase 0، لا يتم fork.

## 7. المعمارية المقترحة

```text
Telegram / iPhone
        │
        ▼
Telegram Gateway (long polling, owner allowlist)
        │
        ▼
Durable Inbox ──► Session Coordinator
                        │
                        ▼
                   Agent Core
                  /     │      \
          Model Router  │   Context Builder
                        │
                 Policy / Approval Layer
                        │
                  Tool Registry
        ┌───────────────┼────────────────────┐
        ▼               ▼                    ▼
 Workspace Tools   Structured Exec       MCP Client
        │               │                    │
        ▼               ▼                    ▼
 Workspace Manager   Git Adapter       Approved MCP servers

 Memory / Scheduler / Task Ledger / Outbox / Audit Log
                        │
                        ▼
                Process Supervisor
                        │
                        ▼
              Android + Termux Runtime
```

المبادئ:

- Telegram private DM لمستخدم رقمي واحد أولًا.
- read-only افتراضيًا.
- لا shell حر؛ تنفيذ argv structured فقط.
- publish ليس وضعًا دائمًا بل موافقة transaction واحدة.
- كل tool يعلن risk level، path/network scope، approval، timeout، idempotency.
- Skills declarative أو MCP منفصل، لا dynamic Go plugins داخل العملية.
- MCP disabled by default، servers pinned وأدواتها allowlisted.
- SQLite/FTS5 دون vector DB في البداية.
- Scheduler يستدعي Skills structured، ولا يجدول shell text.
- Workspace لكل مهمة عبر worktree وفرع `agent/<task-id>`.
- منع write على main/master، ومنع force push والـmerge إلى main.
- web/document inputs بيانات غير موثوقة وليست تعليمات نظام.
- image وInstagram adapters لا تشاركان الأسرار مع Shell أو النموذج.
- Instagram عبر Meta Graph API الرسمي فقط مع exact-content approval.
- audit log يسجل القرار والموافقة والنتيجة بعد redaction.

## 8. تصميم الأمان المقترح

### الأوضاع

- `read-only`: قراءة workspace وGit metadata والويب والذاكرة فقط.
- `write`: لمدة ومهمة محددتين؛ كتابة workspace واختبارات معتمدة.
- `publish`: موافقة أحادية مرتبطة بـcontent hash والحساب والوقت.

### مستويات المخاطر

- R0: قراءة محلية آمنة.
- R1: قراءة شبكة مع SSRF limits.
- R2: patch/write داخل workspace.
- R3: test/commit/image cost.
- R4: push/PR/publish/delete/secrets/MCP مجهول؛ موافقة دائمًا.

### Telegram

- numeric `from.id` فقط، لا username.
- callback approvals مرتبطة بـnonce/session/tool-call hash وصاحب القرار ووقت انتهاء.
- لا تعديل allowlist أو secrets من Telegram.

### Shell

- لا `sh -c`, `bash -c`, eval، pipes أو redirects.
- executable allowlist ومسار resolved.
- environment allowlist، timeout، process group kill، output cap.
- المستودعات غير الموثوقة لا تُشغّل اختبارات محليًا؛ تحتاج sandbox خارجي لاحقًا.

### Prompt injection

- provenance لكل web/doc/repo input.
- المحتوى الخارجي لا يضيف tool أو permission.
- لا mode escalation من ملف أو صفحة.
- لا حفظ تلقائي لمحتوى خارجي في الذاكرة الدائمة.
- الانتقال من read external إلى write/publish يحتاج موافقة.
- SSRF/DNS rebinding/redirect checks.

## 9. Android/Termux

الحزم runtime المقترحة: Git، OpenSSH، CA certificates، termux-services، coreutils، procps، ripgrep، jq، curl للإدارة، وTermux:API اختياري للقياس. لا Python/Node/Rust/Go compiler/Docker في runtime النهائي.

الإشراف المقترح: runit عبر termux-services، Termux:Boot، wake lock، singleton PID/lock، exponential backoff، `svlogd`, health file، crash ledger.

القيود:

- wake lock لا يضمن البقاء على Android/OEMs.
- force-stop قد يمنع BOOT/restart حتى فتح التطبيق.
- proot ليس sandbox أمنيًا.
- local LLM/browser automation غير مناسبين للحرارة والطاقة.

## 10. المخاطر والـRed Flags

1. Android ليس Server OS ولا يقدم SLA.
2. PicoClaw upstream حديث وقد يتغير أو يتوقف.
3. ادعاءات RAM الصغيرة لا تعني صغر المصدر أو الاعتمادات.
4. v0.3.1 يربط عددًا كبيرًا من القنوات والمزودين حتى عند تعطيلها runtime.
5. Shell denylist غير كافٍ بنيويًا.
6. UID واحد في Termux يعني أن chmod/application guards ليست OS containment.
7. Skills/MCP supply-chain surface.
8. crash بعد side effect يحتاج durable ledger/idempotency.
9. Telegram long polling لا يعطي exactly-once تلقائيًا.
10. الجهاز المسروق/rooted/debuggable يعرض الأسرار.
11. Meta/GitHub/image provider terms والتراخيص يجب مراجعتها.
12. كل Skill خارجي يحتاج ترخيصًا مستقلًا؛ لا يفترض أن marketplace كله MIT.
13. إذا تطلب الهدف تشغيل كود غير موثوق على الهاتف، فالخطة غير عملية دون sandbox خارجي.
14. إذا فشل 24-hour/reboot/thermal test، ينتقل القرار إلى NO-GO بدل ترميم المنصة فوق أساس غير موثوق.

## 11. القرار ومستوى الثقة

القرار المسجل:

> **GO مشروط لاختبار PicoClaw v0.3.1 كنقطة انطلاق. لا يوجد اعتماد نهائي، ولا تبدأ Phase 1 حتى نجاح Android Runtime Qualification وفق معايير رقمية.**

سبب عدم اليقين الأكبر هو Android/Termux، ثم غياب العزل الحقيقي، ثم اتساع surface المترابط في PicoClaw.

- الثقة في الاتجاه المعماري: 84%.
- الثقة قبل اختبار الهاتف: 62%.
- الثقة الإجمالية وقت القرار: 78%.

## 12. روابط مرجعية مختصرة

- PicoClaw: https://github.com/sipeed/picoclaw
- Hermes Agent: https://github.com/NousResearch/hermes-agent
- nanobot: https://github.com/HKUDS/nanobot
- OpenClaw: https://github.com/openclaw/openclaw
- PydanticAI: https://github.com/pydantic/pydantic-ai
- LangGraph: https://github.com/langchain-ai/langgraph
- Open Interpreter: https://github.com/openinterpreter/openinterpreter
- OpenHands: https://github.com/OpenHands/OpenHands
- Agent Zero: https://github.com/agent0ai/agent-zero
- AutoGen: https://github.com/microsoft/autogen
- CrewAI: https://github.com/crewAIInc/crewAI
- ZeroClaw: https://github.com/zeroclaw-labs/zeroclaw
- NullClaw: https://github.com/nullclaw/nullclaw
- Moltis: https://github.com/moltis-org/moltis
- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- Termux:Boot: https://github.com/termux/termux-boot
- termux-services: https://github.com/termux/termux-services
