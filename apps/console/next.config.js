/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: 'standalone',
  async rewrites() {
    return [
      // Forward all /api/* calls to the Go backend. The console
      // shares the same backend as the demo (apps/api on :8080)
      // because auth, data, and metrics all live there. The /api
      // prefix is preserved on the destination so the Go
      // server's /api/* routes match (it does NOT mount aliases
      // at /auth/* — the rewrite used to strip /api and every
      // console fetch 404'd).
      { source: '/api/:path*', destination: `${process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'}/api/:path*` }
    ]
  }
}
module.exports = nextConfig
