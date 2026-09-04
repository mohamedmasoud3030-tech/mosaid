-- ============================================================
--  Mosaid Cloud — schema (Supabase / Postgres)
--  ينقل مبادئ "Mosaid" الأساسية إلى السحابة: fail-closed، موافقة
--  فردية مرتبطة بمستخدم+أداة+جزء+مورد، استخدام مرة واحدة،
--  ذاكرة تُقترح ثم تُوافق، وسجل تدقيق.
-- ============================================================

create extension if not exists "uuid-ossp";

-- ------------------------------------------------------------
--  العملاء/المشاريع/الفواتير/المهام/المصاريف (نشاط الفريلانس)
-- ------------------------------------------------------------
create table if not exists public.clients (
  id           uuid primary key default uuid_generate_v4(),
  name         text not null,
  email        text, phone text, whatsapp text, channel text,
  services     text, hourly_rate numeric, budget numeric,
  status       text not null default 'new',
  next_follow_up date, notes text,
  created_at   timestamptz not null default now()
);
create table if not exists public.projects (
  id uuid primary key default uuid_generate_v4(),
  client_id uuid references public.clients(id) on delete set null,
  name text not null, description text,
  status text not null default 'lead',
  value numeric, currency text default 'USD', deadline date,
  deliverables text, created_at timestamptz not null default now()
);
create table if not exists public.invoices (
  id uuid primary key default uuid_generate_v4(),
  number text not null,
  client_id uuid references public.clients(id) on delete set null,
  project_id uuid references public.projects(id) on delete set null,
  amount numeric not null default 0, currency text default 'USD',
  status text not null default 'draft',
  issue_date date, due_date date, notes text,
  created_at timestamptz not null default now()
);
create table if not exists public.tasks (
  id uuid primary key default uuid_generate_v4(),
  title text not null, description text,
  status text not null default 'todo',
  project_id uuid references public.projects(id) on delete set null,
  due_at timestamptz, created_at timestamptz not null default now()
);
create table if not exists public.expenses (
  id uuid primary key default uuid_generate_v4(),
  label text not null, amount numeric not null default 0,
  currency text default 'USD', category text,
  date date default current_date, created_at timestamptz not null default now()
);

-- ------------------------------------------------------------
--  موافقات فردية (نقل من internal/approval)
--  تخزين: token_hash فقط (لا توكن أبداً)، مربوطة بالمستخدم والأداة
--  وجزء الأوامر والموارد، مع TTL وحالة.
-- ------------------------------------------------------------
create table if not exists public.approval_requests (
  id          uuid primary key default uuid_generate_v4(),
  token_hash  text not null unique,
  user_scope  text not null default 'owner',
  tool_name   text not null,
  args_hash   text not null default '',
  resource    text not null default '',
  state       text not null default 'pending',  -- pending|approved|denied|expired
  expires_at  timestamptz not null,
  resolved_at timestamptz,
  created_at  timestamptz not null default now()
);

-- استهلاك الموافقة (الاستخدام مرة واحدة — مطابق لـ approval_uses)
create table if not exists public.approval_uses (
  id          uuid primary key default uuid_generate_v4(),
  approval_id uuid not null references public.approval_requests(id) on delete cascade,
  used_at     timestamptz not null default now()
);

-- ------------------------------------------------------------
--  ذاكرة: مرشّحة -> موافقة -> عنصر مدعوم بـ FTS
-- ------------------------------------------------------------
create table if not exists public.memory_candidates (
  id          uuid primary key default uuid_generate_v4(),
  content     text not null,
  source      text not null default 'owner',
  confidence  numeric not null default 0.5,
  expires_at  timestamptz,
  state       text not null default 'pending', -- pending|approved|denied
  created_at  timestamptz not null default now()
);
create table if not exists public.memory_items (
  id          uuid primary key default uuid_generate_v4(),
  content     text not null,
  source      text not null default 'owner',
  confidence  numeric not null default 0.5,
  expires_at  timestamptz,
  created_at  timestamptz not null default now()
);
-- عمود FTS يدوي (أرسل tsvector بعد إدراج عنصر؛ راجع application.ts)
alter table public.memory_items add column if not exists content_tsv tsvector;
create index if not exists memory_items_fts_idx on public.memory_items
  using gin (content_tsv);

-- ------------------------------------------------------------
--  سجل التدقيق (نقل من internal/audit)
-- ------------------------------------------------------------
create table if not exists public.audit_log (
  id          uuid primary key default uuid_generate_v4(),
  kind        text not null,
  actor       text not null default 'owner',
  resource    text,
  detail      jsonb default '{}'::jsonb,
  decision    text,
  created_at  timestamptz not null default now()
);

-- ------------------------------------------------------------
--  الرسائل/البريد (قابلية إرسال المستقبل — "outbox")
-- ------------------------------------------------------------
create table if not exists public.messages (
  id          uuid primary key default uuid_generate_v4(),
  channel     text not null default 'telegram',
  direction   text not null default 'in',           -- in|out
  body        text not null,
  status      text not null default 'received',     -- received|sent|failed
  external_id text,
  created_at  timestamptz not null default now()
);

-- ------------------------------------------------------------
--  الصلاحيات عبر anon key
-- ------------------------------------------------------------
grant select, insert, update on public.clients to anon, authenticated;
grant select, insert, update on public.projects to anon, authenticated;
grant select, insert, update on public.invoices to anon, authenticated;
grant select, insert, update on public.tasks to anon, authenticated;
grant select, insert, update on public.expenses to anon, authenticated;
grant select, insert, update on public.approval_requests to anon, authenticated;
grant select, insert on public.approval_uses to anon, authenticated;
grant select, insert, update on public.memory_candidates to anon, authenticated;
grant select, insert on public.memory_items to anon, authenticated;
grant select, insert on public.audit_log to anon, authenticated;
grant select, insert, update on public.messages to anon, authenticated;

-- ------------------------------------------------------------
--  دوال Postgres مساعدة (الذاكرة FTS)
-- ------------------------------------------------------------
create or replace function public.memory_set_tsv(memory_id uuid, content text)
returns void language plpgsql security definer set search_path = public as $$
begin
  update public.memory_items
     set content_tsv = to_tsvector('simple', coalesce(content, ''))
   where id = memory_id;
end $$;

create or replace function public.memory_search(query text, max_results int default 20)
returns table (id uuid, content text, source text, confidence numeric, created_at timestamptz)
language plpgsql security definer set search_path = public as $$
begin
  return query
    select m.id, m.content, m.source, m.confidence, m.created_at
      from public.memory_items m
     where m.content_tsv @@ plainto_tsquery('simple', query)
        or m.content ilike '%' || query || '%'
     order by ts_rank_cd(m.content_tsv, plainto_tsquery('simple', query)) desc, m.created_at desc
     limit least(max_results, 50);
end $$;
