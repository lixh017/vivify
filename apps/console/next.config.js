/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: 'standalone',
  async rewrites() {
    return [
      // Forward all /api/* calls to the Go backend. The console
      // shares the same backend as the demo (apps/api on :8080)
      // because auth, data, and metrics all live there.
      { source: '/api/:path*', destination: `${process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'}/:path*` }
    ]
  }
}
module.exports = nextConfig
