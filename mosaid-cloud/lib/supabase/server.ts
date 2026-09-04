import { createServerClient } from "@supabase/ssr";
import { cookies } from "next/headers";

type CookieToSet = { name: string; value: string; options: Record<string, unknown> };

/**
 * عميل Supabase مُهيّأ للعمل مع الكوكيز (الخادم / Server Actions / API Routes).
 * الـ ANON KEY ممكن للقراءة فقط عن طريق Row Level Security الأساسي،
 * حتى لو تسرّب مفتاح ما خسرت شيئاً.
 */
export async function createClient() {
  const cookieStore = await cookies();

  return createServerClient(
    process.env.NEXT_PUBLIC_SUPABASE_URL!,
    process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY!,
    {
      cookies: {
        getAll() {
          return cookieStore.getAll();
        },
        setAll(cookiesToSet: CookieToSet[]) {
          try {
            cookiesToSet.forEach(({ name, value, options }) =>
              cookieStore.set(name, value, options)
            );
          } catch {
            // يُتجاهل في بيئات الخادم الثابتة (البناء)
          }
        },
      },
    }
  );
}
