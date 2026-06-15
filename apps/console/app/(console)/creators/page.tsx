"use client";
import { useEffect, useState } from "react";
import { CreateCreatorDialog } from "./_components/CreateCreatorDialog";
import { ResetPasswordDialog } from "./_components/ResetPasswordDialog";

interface Creator {
    id: number;
    email: string;
    role: string;
    created_by: number;
    created_at: string;
    last_login_at: string | null;
    disabled: boolean;
}

export default function CreatorsPage() {
    const [creators, setCreators] = useState<Creator[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");
    const [createOpen, setCreateOpen] = useState(false);
    const [resetTarget, setResetTarget] = useState<Creator | null>(null);

    async function load() {
        setLoading(true);
        try {
            const res = await fetch("/api/admin/creators", { credentials: "include" });
            if (!res.ok) throw new Error(`load failed: ${res.status}`);
            const data = await res.json();
            setCreators(data.creators);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setLoading(false);
        }
    }

    useEffect(() => { load(); }, []);

    async function disableCreator(c: Creator) {
        if (!confirm(`Disable creator ${c.email}? They won't be able to log in.`)) return;
        const res = await fetch(`/api/admin/creators/${c.id}/disable`, {
            method: "POST", credentials: "include",
        });
        if (!res.ok) { alert("disable failed"); return; }
        await load();
    }

    if (loading) return <p>加载中...</p>;
    if (error) return <p style={{ color: "red" }}>{error}</p>;

    return (
        <div>
            <h1>创作者管理</h1>
            <button onClick={() => setCreateOpen(true)}>+ 创建创作者</button>
            <table>
                <thead>
                    <tr>
                        <th>Email</th><th>创建者</th><th>创建时间</th>
                        <th>最后登录</th><th>状态</th><th>操作</th>
                    </tr>
                </thead>
                <tbody>
                    {creators.map((c) => (
                        <tr key={c.id}>
                            <td>{c.email}</td>
                            <td>{c.created_by || "—"}</td>
                            <td>{c.created_at.slice(0, 19)}</td>
                            <td>{c.last_login_at ? c.last_login_at.slice(0, 19) : "未登录"}</td>
                            <td>{c.disabled ? "已禁用" : "活跃"}</td>
                            <td>
                                <button onClick={() => setResetTarget(c)}>重置密码</button>
                                <button onClick={() => disableCreator(c)} disabled={c.disabled}>
                                    禁用
                                </button>
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>

            {createOpen && (
                <CreateCreatorDialog
                    onClose={() => setCreateOpen(false)}
                    onCreated={() => { setCreateOpen(false); load(); }}
                />
            )}
            {resetTarget && (
                <ResetPasswordDialog
                    creator={resetTarget}
                    onClose={() => setResetTarget(null)}
                    onReset={() => { setResetTarget(null); load(); }}
                />
            )}
        </div>
    );
}