"use client";
import { useState } from "react";

export function CreateAgentDialog({ onClose, onCreated }: {
    onClose: () => void;
    onCreated: () => void;
}) {
    const [name, setName] = useState("");
    const [err, setErr] = useState("");
    const [loading, setLoading] = useState(false);
    const [newKey, setNewKey] = useState<string | null>(null);
    const [confirmed, setConfirmed] = useState(false);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        setErr("");
        setLoading(true);
        const res = await fetch("/api/admin/agents", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            credentials: "include",
            body: JSON.stringify({ name }),
        });
        setLoading(false);
        if (res.ok) {
            const data = await res.json();
            setNewKey(data.key);  // Show 1 time
        } else {
            const body = await res.json().catch(() => ({}));
            setErr(body.error || `failed: ${res.status}`);
        }
    }

    if (newKey) {
        return (
            <div style={overlayStyle}>
                <div style={dialogStyle}>
                    <h2>API key 已生成</h2>
                    <p style={{ color: "red" }}>⚠ 完整 key 仅显示 1 次. 立即复制保存!</p>
                    <pre style={{ background: "#f0f0f0", padding: "0.5rem", overflow: "auto" }}>{newKey}</pre>
                    <label>
                        <input
                            type="checkbox"
                            checked={confirmed}
                            onChange={(e) => setConfirmed(e.target.checked)}
                        />
                        {" "}我已复制保存
                    </label>
                    <br />
                    <button
                        onClick={onCreated}
                        disabled={!confirmed}
                        style={{ marginTop: "1rem" }}
                    >
                        关闭
                    </button>
                </div>
            </div>
        );
    }

    return (
        <div style={overlayStyle}>
            <div style={dialogStyle}>
                <h2>生成新 API key</h2>
                <form onSubmit={submit}>
                    <label>名称 (1-100 字符)
                        <input
                            type="text" value={name}
                            onChange={(e) => setName(e.target.value)}
                            placeholder="例如：claude-code-laptop-1"
                            required minLength={1} maxLength={100}
                            style={{ width: "100%" }}
                        />
                    </label>
                    {err && <p style={{ color: "red" }}>{err}</p>}
                    <button type="submit" disabled={loading}>
                        {loading ? "生成中..." : "生成"}
                    </button>
                    <button type="button" onClick={onClose}>取消</button>
                </form>
            </div>
        </div>
    );
}

const overlayStyle: React.CSSProperties = {
    position: "fixed", top: 0, left: 0, right: 0, bottom: 0,
    background: "rgba(0,0,0,0.5)", display: "flex",
    alignItems: "center", justifyContent: "center", zIndex: 1000,
};
const dialogStyle: React.CSSProperties = {
    background: "white", padding: "1.5rem", borderRadius: "8px",
    minWidth: "500px", maxWidth: "90vw",
};
