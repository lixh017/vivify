"use client";
import { useEffect, useState } from "react";
import { CreateAgentDialog } from "./_components/CreateAgentDialog";

interface Agent {
    id: number;
    name: string;
    key_prefix: string;
    scope: string;
    disabled: boolean;
    created_by: number;
    created_at: string;
    last_used_at: string | null;
}

export default function ApiKeysPage() {
    const [agents, setAgents] = useState<Agent[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");
    const [createOpen, setCreateOpen] = useState(false);

    async function load() {
        setLoading(true);
        try {
            const res = await fetch("/api/admin/agents", { credentials: "include" });
            if (!res.ok) throw new Error(`load failed: ${res.status}`);
            const data = await res.json();
            setAgents(data.agents);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setLoading(false);
        }
    }

    useEffect(() => { load(); }, []);

    async function disableAgent(a: Agent) {
        if (!confirm(`Disable agent ${a.name} (${a.key_prefix}...)? It will no longer be able to call the API.`)) return;
        const res = await fetch(`/api/admin/agents/${a.id}/disable`, {
            method: "POST", credentials: "include",
        });
        if (!res.ok) { alert("disable failed"); return; }
        await load();
    }

    async function regenerateAgent(a: Agent) {
        if (!confirm(`Regenerate API key for ${a.name}? The current key will be invalidated immediately.`)) return;
        const res = await fetch(`/api/admin/agents/${a.id}/regenerate`, {
            method: "POST", credentials: "include",
        });
        if (!res.ok) { alert("regenerate failed"); return; }
        const data = await res.json();
        // Show the new key one time in a confirm dialog (manual copy)
        prompt("New API key (copy now, shown only once):", data.new_key);
        await load();
    }

    if (loading) return <p>加载中...</p>;
    if (error) return <p style={{ color: "red" }}>{error}</p>;

    return (
        <div>
            <h1>API key 管理</h1>
            <p style={{ color: "#666" }}>
                Agent API keys 让非-人类用户（Claude Code / 自研 agent / MCP 客户端）能通过 X-API-Key header 调平台。
            </p>
            <button onClick={() => setCreateOpen(true)}>+ 生成新 API key</button>
            <table>
                <thead>
                    <tr>
                        <th>名称</th><th>Key 前缀</th><th>Scope</th>
                        <th>创建者</th><th>创建时间</th><th>最后使用</th>
                        <th>状态</th><th>操作</th>
                    </tr>
                </thead>
                <tbody>
                    {agents.map((a) => (
                        <tr key={a.id}>
                            <td>{a.name}</td>
                            <td><code>{a.key_prefix}...</code></td>
                            <td>{a.scope}</td>
                            <td>{a.created_by || "—"}</td>
                            <td>{a.created_at.slice(0, 19)}</td>
                            <td>{a.last_used_at ? a.last_used_at.slice(0, 19) : "未使用"}</td>
                            <td>{a.disabled ? "已禁用" : "活跃"}</td>
                            <td>
                                <button onClick={() => regenerateAgent(a)}>重新生成</button>
                                <button onClick={() => disableAgent(a)} disabled={a.disabled}>
                                    禁用
                                </button>
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>

            {createOpen && (
                <CreateAgentDialog
                    onClose={() => setCreateOpen(false)}
                    onCreated={() => { setCreateOpen(false); load(); }}
                />
            )}
        </div>
    );
}
