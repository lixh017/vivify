"use client";
import { useState } from "react";

export function CreateCreatorDialog({ onClose, onCreated }: {
    onClose: () => void;
    onCreated: () => void;
}) {
    const [email, setEmail] = useState("");
    const [password, setPassword] = useState("");
    const [err, setErr] = useState("");
    const [loading, setLoading] = useState(false);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        setErr("");
        setLoading(true);
        const res = await fetch("/api/admin/creators", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            credentials: "include",
            body: JSON.stringify({ email, password }),
        });
        setLoading(false);
        if (res.ok) {
            onCreated();
        } else {
            const body = await res.json().catch(() => ({}));
            setErr(body.error || `failed: ${res.status}`);
        }
    }

    return (
        <div style={overlayStyle}>
            <div style={dialogStyle}>
                <h2>创建创作者</h2>
                <form onSubmit={submit}>
                    <label>Email
                        <input type="email" value={email}
                            onChange={(e) => setEmail(e.target.value)} required />
                    </label>
                    <label>初始密码 (至少 8 字符)
                        <input type="password" value={password}
                            onChange={(e) => setPassword(e.target.value)} required minLength={8} />
                    </label>
                    {err && <p style={{ color: "red" }}>{err}</p>}
                    <button type="submit" disabled={loading}>
                        {loading ? "创建中..." : "创建"}
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
    minWidth: "400px",
};