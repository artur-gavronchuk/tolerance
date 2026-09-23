/** @type {import('next').NextConfig} */
const nextConfig = {
  // Self-contained build for the Docker image (frontend/Dockerfile).
  output: 'standalone',
  typescript: {
    ignoreBuildErrors: true,
  },
  images: {
    unoptimized: true,
  },
}

export default nextConfig
