import { NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "opc_session";
const PUBLIC_PATHS = ["/login"];

export function middleware(req: NextRequest) {
    const { pathname } = req.nextUrl;
    if (PUBLIC_PATHS.includes(pathname)) {
        return NextResponse.next();
    }
    const session = req.cookies.get(SESSION_COOKIE);
    if (!session) {
        const url = req.nextUrl.clone();
        url.pathname = "/login";
        url.searchParams.set("next", pathname);
        return NextResponse.redirect(url);
    }
    return NextResponse.next();
}

export const config = {
    matcher: ["/((?!api|_next/static|_next/image|favicon.ico).*)"],
};
