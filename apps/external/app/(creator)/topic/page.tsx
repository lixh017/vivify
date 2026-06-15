"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { generateTopics, type Topic } from "@/lib/api";

const PLATFORMS = ["抖音", "哔哩哔哩", "小红书"];

export default function TopicPage() {
    const router = useRouter();
    const [seed, setSeed] = useState("");
    const [platform, setPlatform] = useState("抖音");
    const [count, setCount] = useState(5);
    const [topics, setTopics] = useState<Topic[] | null>(null);
    const [err, setErr] = useState("");
    const [loading, setLoading] = useState(false);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        setErr("");
        setTopics(null);
        setLoading(true);
        try {
            const t = await generateTopics({ seed, platform, count });
            setTopics(t);
        } catch (e) {
            setErr((e as Error).message);
        } finally {
            setLoading(false);
        }
    }

    return (
        <main style={{ maxWidth: 800, margin: "2rem auto", padding: "1.5rem" }}>
            <button onClick={() => router.push("/dashboard")}>← 返回</button>
            <h1>新建选题</h1>

            <form onSubmit={submit} style={{ marginTop: "1rem" }}>
                <div style={{ marginBottom: "1rem" }}>
                    <label>Seed (必填)<br />
                        <input
                            type="text" value={seed}
                            onChange={(e) => setSeed(e.target.value)}
                            placeholder="例如：个人成长"
                            required
                            style={{ width: "100%" }}
                        />
                    </label>
                </div>
                <div style={{ marginBottom: "1rem" }}>
                    <label>平台<br />
                        <select value={platform} onChange={(e) => setPlatform(e.target.value)}>
                            {PLATFORMS.map((p) => <option key={p} value={p}>{p}</option>)}
                        </select>
                    </label>
                </div>
                <div style={{ marginBottom: "1rem" }}>
                    <label>数量 (1-20)<br />
                        <input
                            type="number" min={1} max={20}
                            value={count} onChange={(e) => setCount(parseInt(e.target.value) || 5)}
                            style={{ width: "100px" }}
                        />
                    </label>
                </div>
                <p style={{ color: "#888", fontSize: "0.9em" }}>
                    IP profile: anthropomorphic (峰哥) — M1 固定
                </p>
                <button type="submit" disabled={loading}>
                    {loading ? "生成中..." : "生成"}
                </button>
            </form>

            {err && <p style={{ color: "red", marginTop: "1rem" }}>{err}</p>}

            {topics && (
                <section style={{ marginTop: "2rem" }}>
                    <h2>结果 ({topics.length} 个选题)</h2>
                    {topics.map((t, i) => (
                        <article key={i} style={{ background: "white", padding: "1rem", marginBottom: "0.5rem", borderRadius: 4, boxShadow: "0 1px 3px rgba(0,0,0,0.08)" }}>
                            <h3 style={{ margin: "0 0 0.5rem 0" }}>{i + 1}. {t.title}</h3>
                            <p style={{ margin: "0.25rem 0" }}><b>角度:</b> {t.angle}</p>
                            <p style={{ margin: "0.25rem 0" }}><b>Hook:</b> {t.hook}</p>
                            <p style={{ margin: "0.25rem 0" }}><b>预期表现:</b> {t.expected_performance}</p>
                        </article>
                    ))}
                </section>
            )}
        </main>
    );
}
