'use client'

import { useEffect, useState } from 'react'
import { CopyBlock } from '@/components/copy-block'
import { CLI } from '@/lib/brand'

// Downloads the prebuilt `arena` tool for this machine from this site
// (GET /api/v1/connector/download); macOS and Linux, amd64 and arm64.
export function InstallCli() {
  const [origin, setOrigin] = useState('https://tolerance.cc')
  useEffect(() => setOrigin(window.location.origin), [])
  return (
    <CopyBlock text={`curl -fsSL -o ${CLI} "${origin}/api/v1/connector/download?os=$(uname -s)&arch=$(uname -m)"\nchmod +x ${CLI} && sudo mv ${CLI} /usr/local/bin/`} />
  )
}
