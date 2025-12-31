// Root layout - wraps all pages with ThemeProvider, AuthProvider, Toaster, and global styles
import type { Metadata } from "next";
import "./globals.css";
import { ThemeProvider } from "@/lib/theme";
import { Toaster } from "@/components/ui/sonner";
import { ScrollToTop } from "@/components/ScrollToTop";
import { AuthProvider } from "@/components/AuthProvider";

export const metadata: Metadata = {
  title: "Docklift - Self-hosted Docker Deployment Platform",
  description: "Open-source, self-hosted PaaS for Docker deployments. Deploy any application to your own server with one click. Free alternative to Heroku, Vercel, Netlify.",
  keywords: ["docker", "deployment", "self-hosted", "paas", "devops", "containers", "open-source"],
  authors: [{ name: "SSujitX" }],
  robots: {
    index: false,
    follow: false,
    googleBot: {
      index: false,
      follow: false,
    },
  },
  openGraph: {
    title: "Docklift - Self-hosted Docker Deployment Platform",
    description: "Deploy any application to your own server with one click. Open-source, self-hosted PaaS.",
    type: "website",
    url: "https://github.com/SSujitX/docklift",
  },
  twitter: {
    card: "summary_large_image",
    title: "Docklift - Self-hosted Docker Deployment Platform",
    description: "Deploy any application to your own server with one click.",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="min-h-screen antialiased">
        <ThemeProvider>
          <AuthProvider>
            <div className="relative min-h-screen flex flex-col">
              <div className="fixed inset-0 -z-10 bg-[radial-gradient(ellipse_80%_80%_at_50%_-20%,rgba(56,189,248,0.08),transparent)] dark:bg-[radial-gradient(ellipse_80%_80%_at_50%_-20%,rgba(56,189,248,0.12),transparent)]" />
              {children}
              <ScrollToTop />
            </div>
            <Toaster />
          </AuthProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}

