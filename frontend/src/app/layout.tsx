import type { Metadata } from "next";
import { cookies } from "next/headers";
import { Inter } from "next/font/google";
import "./globals.css";

import { NuqsAdapter } from "nuqs/adapters/next/app";
import { ThemeProvider } from "next-themes";

import { AuthProvider } from "@/contexts/AuthContext";
import ReactQueryProvider from "@/contexts/ReactQueryProvider";
import { Toaster } from "@/components/ui/sonner";
import { I18nProvider } from "@/i18n/I18nProvider";
import { LOCALE_COOKIE, parseLocale, translate } from "@/i18n/translate";

const inter = Inter({ subsets: ["latin"] });

export async function generateMetadata(): Promise<Metadata> {
  const jar = await cookies();
  const locale = parseLocale(jar.get(LOCALE_COOKIE)?.value);
  return {
    title: translate(locale, "site.title"),
    description: translate(locale, "site.description"),
    icons: { icon: "/get_favicon" },
  };
}

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const jar = await cookies();
  const locale = parseLocale(jar.get(LOCALE_COOKIE)?.value);
  return (
    // next-themes stamps the theme class on <html> before paint, which the exported markup
    // cannot predict; suppressHydrationWarning confines that mismatch to this element.
    <html lang={locale} suppressHydrationWarning>
      <body className={inter.className}>
        <I18nProvider initialLocale={locale}>
          <ThemeProvider attribute="class" defaultTheme="light" enableSystem disableTransitionOnChange>
            <NuqsAdapter>
              <ReactQueryProvider>
                <AuthProvider>{children}</AuthProvider>
                <Toaster />
              </ReactQueryProvider>
            </NuqsAdapter>
          </ThemeProvider>
        </I18nProvider>
      </body>
    </html>
  );
}
