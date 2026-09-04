import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "رحلتي — الوكيل الشخصي",
  description: "مساعد فريلانس شخصي يعيش في رحلتك اليومية",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="ar" dir="rtl">
      <body>{children}</body>
    </html>
  );
}
