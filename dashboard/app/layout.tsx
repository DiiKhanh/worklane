import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import { Providers } from "./providers";
import { Toaster } from "@/components/ui/sonner";
import { Sidebar } from "@/components/shell/sidebar";
import { Topbar } from "@/components/shell/topbar";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "worklane - OTP dashboard",
  description: "Developer dashboard for the worklane OTP / verification platform",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full">
        <Providers>
          <div className="flex min-h-dvh">
            <Sidebar />
            <div className="flex min-w-0 flex-1 flex-col">
              <Topbar />
              <main className="flex-1 px-5 py-6 md:px-8 md:py-8">
                <div className="mx-auto w-full max-w-6xl">{children}</div>
              </main>
            </div>
          </div>
        </Providers>
        <Toaster />
      </body>
    </html>
  );
}
