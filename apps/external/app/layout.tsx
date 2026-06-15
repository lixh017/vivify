import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
    title: "OPC 创作者平台",
    description: "External creator beta UI",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
    return (
        <html lang="zh">
            <body style={{ fontFamily: "system-ui, sans-serif", margin: 0, padding: 0 }}>
                {children}
            </body>
        </html>
    );
}
