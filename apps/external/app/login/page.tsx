"use client";
import { Suspense, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { login } from "@/lib/api";

function LoginForm() {
    const router = useRouter();
    const search = useSearchParams();
    const next = search.get("next") || "/dashboard";
    const [email, setEmail] = useState("");
    const [password, setPassword] = useState("");
    const [err, setErr] = useState("");
    const [loading, setLoading] = useState(false);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        setLoading(true);
        setErr("");
        try {
            const user = await login(email, password);
            if (user.role !== "creator") {
                setErr("此账号不是创作者账号");
                setLoading(false);
                return;
            }
            router.push(next);
        } catch (e) {
            setErr((e as Error).message);
            setLoading(false);
        }
    }

    return (
        <main style={{ maxWidth: 400, margin: "5rem auto", padding: "1.5rem", background: "white", borderRadius: 8, boxShadow: "0 2px 8px rgba(0,0,0,0.1)" }}>
            <h1 style={{ marginTop: 0 }}>创作者登录</h1>
            <form onSubmit={submit}>
                <div style={{ marginBottom: "1rem" }}>
                    <label>Email<br />
                        <input
                            type="email" value={email}
                            onChange={(e) => setEmail(e.target.value)}
                            placeholder="creator@beta.com"
                            required
                            style={{ width: "100%" }}
                        />
                    </label>
                </div>
                <div style={{ marginBottom: "1rem" }}>
                    <label>密码<br />
                        <input
                            type="password" value={password}
                            onChange={(e) => setPassword(e.target.value)}
                            placeholder="密码"
                            required
                            style={{ width: "100%" }}
                        />
                    </label>
                </div>
                {err && <p style={{ color: "red" }}>{err}</p>}
                <button type="submit" disabled={loading} style={{ width: "100%" }}>
                    {loading ? "登录中..." : "登录"}
                </button>
            </form>
        </main>
    );
}

export default function LoginPage() {
    return (
        <Suspense fallback={<p style={{ padding: "1rem" }}>加载中...</p>}>
            <LoginForm />
        </Suspense>
    );
}
