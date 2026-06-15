// Thin fetch wrappers for the OPC API. Session cookie is sent
// automatically via credentials: 'include'.

export interface Creator {
    id: number;
    email: string;
    role: string;
    created_at: string;
    last_login_at: string | null;
    disabled: boolean;
}

export interface Topic {
    title: string;
    angle: string;
    expected_performance: string;
    hook: string;
    pattern?: string;
    voice_tags?: string[];
}

export async function login(email: string, password: string): Promise<Creator> {
    const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ email, password }),
    });
    if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || `login failed: ${res.status}`);
    }
    const data = await res.json();
    return data.user;
}

export async function getMe(): Promise<Creator | null> {
    const res = await fetch("/api/auth/me", { credentials: "include" });
    if (res.status === 401) return null;
    if (!res.ok) throw new Error(`me failed: ${res.status}`);
    const data = await res.json();
    return data.user;
}

export async function generateTopics(input: {
    seed: string;
    platform: string;
    count: number;
}): Promise<Topic[]> {
    const res = await fetch("/api/ai/topics", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(input),
    });
    if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || `generate failed: ${res.status}`);
    }
    const data = await res.json();
    return data.topics;
}
