import { readFile, stat } from 'node:fs/promises';
import { basename, extname, join } from 'node:path';
import { currentBranch, headSha, listFiles } from '@studio/git';
import type { LanguageShare, RepoScan, ScanCategory, ScanFinding } from '@studio/schema';

/**
 * Quick, deterministic look at what a repository is made of: languages, frameworks, databases,
 * infrastructure, tests and docs. No LLM; reads manifests and file names only. Takes well under a
 * second on a typical repo, so the project page has something to show before an agent runs.
 */
export async function scanRepo(repo: string): Promise<RepoScan> {
  const [files, commitSha, branch] = await Promise.all([listFiles(repo), headSha(repo), currentBranch(repo)]);
  const findings = new Findings();

  await Promise.all([
    ...files.filter((f) => basename(f) === 'package.json').map((f) => scanPackageJson(repo, f, findings)),
    ...files.filter((f) => basename(f) === 'go.mod').map((f) => scanByContent(repo, f, GO_RULES, findings)),
    ...files
      .filter((f) => ['requirements.txt', 'pyproject.toml', 'Pipfile'].includes(basename(f)))
      .map((f) => scanByContent(repo, f, PYTHON_RULES, findings)),
    ...files
      .filter((f) => basename(f) === 'Cargo.toml')
      .map((f) => scanByContent(repo, f, RUST_RULES, findings)),
    ...files
      .filter((f) => ['pom.xml', 'build.gradle', 'build.gradle.kts'].includes(basename(f)))
      .map((f) => scanByContent(repo, f, JVM_RULES, findings)),
    ...files
      .filter((f) => basename(f) === 'Gemfile')
      .map((f) => scanByContent(repo, f, RUBY_RULES, findings)),
    ...files
      .filter((f) => basename(f) === 'composer.json')
      .map((f) => scanByContent(repo, f, PHP_RULES, findings)),
    ...files.filter(isComposeFile).map((f) => scanByContent(repo, f, COMPOSE_RULES, findings)),
  ]);
  scanFileNames(files, findings);

  return {
    commitSha,
    branch,
    totalFiles: files.length,
    languages: await languageShares(repo, files),
    findings: findings.list(),
  };
}

// ── findings ────────────────────────────────────────────────────────────────

class Findings {
  private readonly byKey = new Map<string, ScanFinding>();

  add(category: ScanCategory, label: string, evidence: string, generic = false): void {
    const key = `${category}:${label}`;
    const existing = this.byKey.get(key);
    if (!existing) this.byKey.set(key, { category, label, generic, count: 1, evidence: [evidence] });
    else if (!existing.evidence.includes(evidence)) {
      existing.evidence.push(evidence);
      existing.count = existing.evidence.length;
    }
  }

  addMany(category: ScanCategory, label: string, files: string[]): void {
    this.byKey.set(`${category}:${label}`, {
      category,
      label,
      generic: true,
      count: files.length,
      evidence: files.slice(0, MAX_NAME_EVIDENCE),
    });
  }

  list(): ScanFinding[] {
    return [...this.byKey.values()]
      .map((f) => ({ ...f, evidence: f.evidence.sort() }))
      .sort((a, b) => a.category.localeCompare(b.category) || a.label.localeCompare(b.label));
  }
}

type Rule = { match: RegExp; category: ScanCategory; label: string; generic?: boolean };

// ── package.json ────────────────────────────────────────────────────────────

const NPM_RULES: Record<string, { category: ScanCategory; label: string }> = {
  next: { category: 'frontend', label: 'Next.js' },
  react: { category: 'frontend', label: 'React' },
  vue: { category: 'frontend', label: 'Vue' },
  nuxt: { category: 'frontend', label: 'Nuxt' },
  svelte: { category: 'frontend', label: 'Svelte' },
  '@sveltejs/kit': { category: 'frontend', label: 'SvelteKit' },
  '@angular/core': { category: 'frontend', label: 'Angular' },
  'solid-js': { category: 'frontend', label: 'Solid' },
  astro: { category: 'frontend', label: 'Astro' },
  vite: { category: 'frontend', label: 'Vite' },
  tailwindcss: { category: 'frontend', label: 'Tailwind CSS' },
  electron: { category: 'frontend', label: 'Electron' },
  'react-native': { category: 'frontend', label: 'React Native' },
  express: { category: 'backend', label: 'Express' },
  fastify: { category: 'backend', label: 'Fastify' },
  '@nestjs/core': { category: 'backend', label: 'NestJS' },
  hono: { category: 'backend', label: 'Hono' },
  koa: { category: 'backend', label: 'Koa' },
  '@trpc/server': { category: 'backend', label: 'tRPC' },
  graphql: { category: 'backend', label: 'GraphQL' },
  prisma: { category: 'database', label: 'Prisma' },
  '@prisma/client': { category: 'database', label: 'Prisma' },
  'drizzle-orm': { category: 'database', label: 'Drizzle ORM' },
  typeorm: { category: 'database', label: 'TypeORM' },
  sequelize: { category: 'database', label: 'Sequelize' },
  mongoose: { category: 'database', label: 'MongoDB (Mongoose)' },
  mongodb: { category: 'database', label: 'MongoDB' },
  pg: { category: 'database', label: 'PostgreSQL' },
  postgres: { category: 'database', label: 'PostgreSQL' },
  mysql2: { category: 'database', label: 'MySQL' },
  'better-sqlite3': { category: 'database', label: 'SQLite' },
  redis: { category: 'database', label: 'Redis' },
  ioredis: { category: 'database', label: 'Redis' },
  '@supabase/supabase-js': { category: 'database', label: 'Supabase' },
  vitest: { category: 'testing', label: 'Vitest' },
  jest: { category: 'testing', label: 'Jest' },
  '@playwright/test': { category: 'testing', label: 'Playwright' },
  cypress: { category: 'testing', label: 'Cypress' },
  mocha: { category: 'testing', label: 'Mocha' },
};

async function scanPackageJson(repo: string, file: string, findings: Findings): Promise<void> {
  const text = await readText(repo, file);
  if (!text) return;
  let pkg: { dependencies?: Record<string, string>; devDependencies?: Record<string, string> };
  try {
    pkg = JSON.parse(text);
  } catch {
    return;
  }
  const deps = { ...pkg.dependencies, ...pkg.devDependencies };
  for (const [name, version] of Object.entries(deps)) {
    const rule = NPM_RULES[name];
    if (rule) findings.add(rule.category, withMajor(rule.label, version), file);
  }
}

/** "Next.js" + "^15.2.0" → "Next.js 15"; ranges like "workspace:*" or "latest" give just the label. */
function withMajor(label: string, version: string): string {
  const major = /^[\^~>=v ]*(\d+)\./.exec(version)?.[1];
  return major ? `${label} ${major}` : label;
}

// ── other manifests (matched by content) ────────────────────────────────────

const GO_RULES: Rule[] = [
  { match: /^go \d/m, category: 'backend', label: 'Go' },
  { match: /github\.com\/gin-gonic\/gin/, category: 'backend', label: 'Gin' },
  { match: /github\.com\/labstack\/echo/, category: 'backend', label: 'Echo' },
  { match: /github\.com\/go-chi\/chi/, category: 'backend', label: 'chi' },
  { match: /github\.com\/gofiber\/fiber/, category: 'backend', label: 'Fiber' },
  { match: /google\.golang\.org\/grpc/, category: 'backend', label: 'gRPC' },
  { match: /github\.com\/jackc\/pgx/, category: 'database', label: 'PostgreSQL' },
  { match: /github\.com\/lib\/pq/, category: 'database', label: 'PostgreSQL' },
  { match: /gorm\.io\/gorm/, category: 'database', label: 'GORM' },
  { match: /github\.com\/go-sql-driver\/mysql/, category: 'database', label: 'MySQL' },
  { match: /modernc\.org\/sqlite|github\.com\/mattn\/go-sqlite3/, category: 'database', label: 'SQLite' },
  {
    match: /github\.com\/redis\/go-redis|github\.com\/go-redis\/redis/,
    category: 'database',
    label: 'Redis',
  },
  { match: /go\.mongodb\.org\/mongo-driver/, category: 'database', label: 'MongoDB' },
  {
    match: /github\.com\/pressly\/goose|github\.com\/golang-migrate\/migrate/,
    category: 'database',
    label: 'sqlMigrations',
    generic: true,
  },
  { match: /github\.com\/docker\/docker|github\.com\/moby\/moby/, category: 'infra', label: 'Docker API' },
];

const PYTHON_RULES: Rule[] = [
  { match: /.?/, category: 'backend', label: 'Python' },
  { match: /\bdjango\b/i, category: 'backend', label: 'Django' },
  { match: /\bflask\b/i, category: 'backend', label: 'Flask' },
  { match: /\bfastapi\b/i, category: 'backend', label: 'FastAPI' },
  { match: /\bcelery\b/i, category: 'backend', label: 'Celery' },
  { match: /\bsqlalchemy\b/i, category: 'database', label: 'SQLAlchemy' },
  { match: /\balembic\b/i, category: 'database', label: 'Alembic' },
  { match: /\bpsycopg/i, category: 'database', label: 'PostgreSQL' },
  { match: /\bpytest\b/i, category: 'testing', label: 'pytest' },
];

const RUST_RULES: Rule[] = [
  { match: /\[package\]/, category: 'backend', label: 'Rust' },
  { match: /^axum\s*=/m, category: 'backend', label: 'axum' },
  { match: /^actix-web\s*=/m, category: 'backend', label: 'Actix Web' },
  { match: /^sqlx\s*=/m, category: 'database', label: 'SQLx' },
  { match: /^diesel\s*=/m, category: 'database', label: 'Diesel' },
  { match: /^tauri\s*=/m, category: 'frontend', label: 'Tauri' },
];

const JVM_RULES: Rule[] = [
  { match: /.?/, category: 'backend', label: 'JVM' },
  { match: /spring-boot/, category: 'backend', label: 'Spring Boot' },
  { match: /ktor/, category: 'backend', label: 'Ktor' },
  { match: /hibernate|spring-boot-starter-data-jpa/, category: 'database', label: 'JPA / Hibernate' },
  { match: /flyway/, category: 'database', label: 'Flyway' },
  { match: /liquibase/, category: 'database', label: 'Liquibase' },
  { match: /junit/, category: 'testing', label: 'JUnit' },
];

const RUBY_RULES: Rule[] = [
  { match: /.?/, category: 'backend', label: 'Ruby' },
  { match: /gem ['"]rails['"]/, category: 'backend', label: 'Ruby on Rails' },
  { match: /gem ['"]rspec/, category: 'testing', label: 'RSpec' },
];

const PHP_RULES: Rule[] = [
  { match: /.?/, category: 'backend', label: 'PHP' },
  { match: /laravel\/framework/, category: 'backend', label: 'Laravel' },
  { match: /symfony\//, category: 'backend', label: 'Symfony' },
];

const COMPOSE_RULES: Rule[] = [
  { match: /.?/, category: 'infra', label: 'Docker Compose' },
  { match: /image:\s*["']?(\S+\/)?postgres/, category: 'database', label: 'PostgreSQL' },
  { match: /image:\s*["']?(\S+\/)?(mysql|mariadb)/, category: 'database', label: 'MySQL' },
  { match: /image:\s*["']?(\S+\/)?mongo/, category: 'database', label: 'MongoDB' },
  { match: /image:\s*["']?(\S+\/)?(redis|valkey)/, category: 'database', label: 'Redis' },
  { match: /image:\s*["']?(\S+\/)?clickhouse/, category: 'database', label: 'ClickHouse' },
  { match: /image:\s*["']?(\S+\/)?(cp-kafka|kafka|redpanda)/, category: 'infra', label: 'Kafka' },
  { match: /image:\s*["']?(\S+\/)?rabbitmq/, category: 'infra', label: 'RabbitMQ' },
  { match: /image:\s*["']?(\S+\/)?(nginx|caddy|traefik)/, category: 'infra', label: 'Reverse proxy' },
];

function isComposeFile(file: string): boolean {
  return /(^|\/)(docker-)?compose(\.[\w-]+)?\.ya?ml$/.test(file);
}

async function scanByContent(repo: string, file: string, rules: Rule[], findings: Findings): Promise<void> {
  const text = await readText(repo, file);
  if (text === null) return;
  for (const rule of rules)
    if (rule.match.test(text)) findings.add(rule.category, rule.label, file, rule.generic);
}

// ── file names ──────────────────────────────────────────────────────────────

const NAME_RULES: { test: (file: string) => boolean; category: ScanCategory; label: string }[] = [
  { test: (f) => /(^|\/)migrations?\/.+\.sql$/i.test(f), category: 'database', label: 'sqlMigrations' },
  { test: (f) => /(^|\/)schema\.prisma$/.test(f), category: 'database', label: 'prismaSchema' },
  { test: (f) => /\.sql$/i.test(f) && !/migrations?\//i.test(f), category: 'database', label: 'sqlFiles' },
  { test: (f) => /(^|\/)Dockerfile[^/]*$/.test(f), category: 'infra', label: 'dockerfiles' },
  { test: (f) => /^\.github\/workflows\/.+\.ya?ml$/.test(f), category: 'infra', label: 'githubActions' },
  { test: (f) => f === '.gitlab-ci.yml', category: 'infra', label: 'gitlabCi' },
  { test: (f) => /(^|\/)Chart\.yaml$/.test(f), category: 'infra', label: 'helm' },
  { test: (f) => /\.tf$/.test(f), category: 'infra', label: 'terraform' },
  { test: (f) => /(^|\/)Caddyfile$/.test(f), category: 'infra', label: 'caddy' },
  { test: (f) => /(^|\/)nginx[^/]*\.conf$/.test(f), category: 'infra', label: 'nginx' },
  { test: (f) => f === 'Makefile', category: 'infra', label: 'makefile' },
  { test: (f) => /_test\.go$/.test(f), category: 'testing', label: 'goTests' },
  { test: (f) => /\.(test|spec)\.[cm]?[jt]sx?$/.test(f), category: 'testing', label: 'jsTests' },
  { test: (f) => /(^|\/)test_[^/]+\.py$|_test\.py$/.test(f), category: 'testing', label: 'pythonTests' },
  { test: (f) => /(^|\/)README[^/]*\.md$/i.test(f), category: 'docs', label: 'readme' },
  { test: (f) => /^docs?\/.+\.(md|mdx|rst)$/i.test(f), category: 'docs', label: 'docsFolder' },
  { test: (f) => /(^|\/)(CLAUDE|AGENTS)\.md$/.test(f), category: 'docs', label: 'agentInstructions' },
  {
    test: (f) => /(^|\/)openapi[^/]*\.(ya?ml|json)$|(^|\/)swagger[^/]*\.(ya?ml|json)$/i.test(f),
    category: 'backend',
    label: 'openapi',
  },
  { test: (f) => /\.proto$/.test(f), category: 'backend', label: 'protobuf' },
];

/** At most this many evidence files per name rule; the count matters more than the full list. */
const MAX_NAME_EVIDENCE = 12;

function scanFileNames(files: string[], findings: Findings): void {
  for (const rule of NAME_RULES) {
    const matched = files.filter(rule.test);
    if (matched.length > 0) findings.addMany(rule.category, rule.label, matched);
  }
}

// ── languages ───────────────────────────────────────────────────────────────

const LANGUAGES: Record<string, string> = {
  '.ts': 'TypeScript',
  '.tsx': 'TypeScript',
  '.mts': 'TypeScript',
  '.cts': 'TypeScript',
  '.js': 'JavaScript',
  '.jsx': 'JavaScript',
  '.mjs': 'JavaScript',
  '.cjs': 'JavaScript',
  '.go': 'Go',
  '.py': 'Python',
  '.rs': 'Rust',
  '.java': 'Java',
  '.kt': 'Kotlin',
  '.kts': 'Kotlin',
  '.scala': 'Scala',
  '.rb': 'Ruby',
  '.php': 'PHP',
  '.cs': 'C#',
  '.swift': 'Swift',
  '.c': 'C',
  '.h': 'C',
  '.cpp': 'C++',
  '.cc': 'C++',
  '.hpp': 'C++',
  '.vue': 'Vue',
  '.svelte': 'Svelte',
  '.css': 'CSS',
  '.scss': 'CSS',
  '.html': 'HTML',
  '.sql': 'SQL',
  '.sh': 'Shell',
  '.dart': 'Dart',
  '.ex': 'Elixir',
  '.exs': 'Elixir',
  '.lua': 'Lua',
};

async function languageShares(repo: string, files: string[]): Promise<LanguageShare[]> {
  const totals = new Map<string, { files: number; bytes: number }>();
  const sources = files.filter((f) => LANGUAGES[extname(f).toLowerCase()] && !isVendored(f));
  await Promise.all(
    sources.map(async (f) => {
      const size = await stat(join(repo, f)).then(
        (s) => s.size,
        () => 0,
      );
      const name = LANGUAGES[extname(f).toLowerCase()] ?? 'Other';
      const t = totals.get(name) ?? { files: 0, bytes: 0 };
      t.files += 1;
      t.bytes += size;
      totals.set(name, t);
    }),
  );
  const all = [...totals.values()].reduce((sum, t) => sum + t.bytes, 0) || 1;
  return [...totals.entries()]
    .map(([name, t]) => ({ name, ...t, share: t.bytes / all }))
    .sort((a, b) => b.bytes - a.bytes);
}

function isVendored(file: string): boolean {
  return (
    /(^|\/)(node_modules|vendor|dist|build|out|\.next|third_party)\//.test(file) || /\.min\.js$/.test(file)
  );
}

async function readText(repo: string, file: string): Promise<string | null> {
  return readFile(join(repo, file), 'utf8').catch(() => null);
}
