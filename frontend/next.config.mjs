/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  // The browser always talks to /api on the site's own origin; Next proxies
  // it to the Go API so the session cookie is first-party everywhere.
  async rewrites() {
    const api = process.env.API_URL ?? 'http://127.0.0.1:8080'
    return [{ source: '/api/:path*', destination: `${api}/api/:path*` }]
  },
  async headers() {
    return [
      {
        source: '/:path*',
        headers: [
          { key: 'X-Content-Type-Options', value: 'nosniff' },
          { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
          { key: 'X-Frame-Options', value: 'SAMEORIGIN' },
          { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=()' },
        ],
      },
    ]
  },
}

export default nextConfig
