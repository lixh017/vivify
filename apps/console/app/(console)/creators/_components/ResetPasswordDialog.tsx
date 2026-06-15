"use client";
import { useState } from "react";

interface Creator {
    id: number;
    email: string;
}

export function ResetPasswordDialog({ creator, onClose, onReset }: {
    creator: Creator;
    onClose: () => void;
    onReset: () => void;
}) {
    const [newPassword, setNewPassword] = useState("");
    const [err, setErr] = useState("");
    const [loading, setLoading] = useState(false);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        setErr("");
        setLoading(true);
        const res = await fetch(`/api/admin/creators/${creator.id}/reset-password`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            credentials: "include",
            body: JSON.stringify({ new_password: newPassword }),
        });
        setLoading(false);
        if (res.ok) {
            onReset();
        } else {
            const body = await res.json().catch(() => ({}));
            setErr(body.error || `failed: ${res.status}`);
        }
    }

    return (
        <div style={overlayStyle}>
            <div style={dialogStyle}>
                <h2>重置密码: {creator.email}</h2>
                <form onSubmit={submit}>
                    <label>新密码 (至少 8 字符)
                        <input type="password" value={newPassword}
                            onChange={(e) => setNewPassword(e.target.value)} required minLength={8} />
                    </label>
                    {err && <p style={{ color: "red" }}>{err}</p>}
                    <button type="submit" disabled={loading}>
                        {loading ? "重置中..." : "重置"}
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