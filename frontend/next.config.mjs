/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  // The browser always talks to /api on the site's own origin; Next proxies
  // it to the Go API so the session cookie is first-party everywhere.
  // Only the build challenge is open for now (since 2026-10-05); the other modes are hidden, not deleted.
  async redirects() {
    return ['/build', '/day/:path*', '/days', '/leaderboard', '/tanks', '/tanks/:path*', '/app/tanks', '/app/tanks/:path*',
      '/docs', '/docs/:path*', '/u/:path*'].map((source) => ({ source, destination: '/', permanent: false }))
  },
  async rewrites() {
    const api = process.env.API_URL ?? 'http://127.0.0.1:8080'
    return [{ source: '/api/:path*', destination: `${api}/api/:path*` }]
  },
}

export default nextConfig
