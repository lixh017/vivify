import { redirect } from "next/navigation";
import { getMe } from "@/lib/api";

// Opt out of static prerendering — getMe() calls a relative
// /api/auth/me which has no base URL at build time.
export const dynamic = "force-dynamic";

export default async function RootPage() {
    const me = await getMe();
    if (me) {
        redirect("/dashboard");
    } else {
        redirect("/login");
    }
}
