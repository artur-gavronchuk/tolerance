#!/usr/bin/env node
// Fake `claude -p --output-format stream-json`: replays the JSON lines in $FAKE_CLAUDE_SCRIPT,
// replacing "$CWD" with the working directory, and records its arguments to $FAKE_CLAUDE_ARGS.
import { appendFileSync, readFileSync } from 'node:fs';

if (process.env.FAKE_CLAUDE_ARGS)
  appendFileSync(process.env.FAKE_CLAUDE_ARGS, `${JSON.stringify(process.argv.slice(2))}\n`);
const lines = JSON.parse(readFileSync(process.env.FAKE_CLAUDE_SCRIPT, 'utf8'));
const runIndex = Number(process.env.FAKE_CLAUDE_RUN ?? 0);
const script = Array.isArray(lines[0]) ? (lines[Math.min(runIndex, lines.length - 1)] ?? []) : lines;
for (const line of script)
  process.stdout.write(`${JSON.stringify(line).replaceAll('$CWD', process.cwd())}\n`);
process.exit(Number(process.env.FAKE_CLAUDE_EXIT ?? 0));
