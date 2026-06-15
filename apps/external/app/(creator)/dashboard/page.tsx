"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { getMe } from "@/lib/api";

export default function DashboardPage() {
    const router = useRouter();
    const [me, setMe] = useState<{ email: string; role: string } | null>(null);

    useEffect(() => {
        getMe().then((u) => {
            if (!u) router.push("/login");
            else setMe(u);
        });
    }, [router]);

    async function logout() {
        await fetch("/api/auth/logout", { method: "POST", credentials: "include" });
        router.push("/login");
    }

    if (!me) return <p style={{ padding: "1rem" }}>加载中...</p>;

    return (
        <main style={{ maxWidth: 800, margin: "2rem auto", padding: "1.5rem" }}>
            <header style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <h1>创作者 Dashboard</h1>
                <div>
                    <span>{me.email} ({me.role})</span>{" "}
                    <button onClick={logout}>登出</button>
                </div>
            </header>

            <section style={{ marginTop: "2rem" }}>
                <h2>选题</h2>
                <p style={{ color: "#666" }}>
                    生成 5 个选题，基于 OPC 品牌 IP profile (anthropomorphic / 峰哥 panda)。
                </p>
                <button
                    onClick={() => router.push("/topic")}
                    style={{ padding: "0.75rem 1.5rem", fontSize: "1rem" }}
                >
                    + 新建选题
                </button>
            </section>

            <section style={{ marginTop: "2rem" }}>
                <h2>历史</h2>
                <p style={{ color: "#888", fontStyle: "italic" }}>
                    M1 暂未持久化选题历史；你的选题记录在 OPC operator 的 observability dashboard
                    中可查询（含 cost_cents 标记）。
                </p>
            </section>
        </main>
    );
}
