import {
  chmodSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  branchState,
  commitWorkspace,
  createPullRequest,
  createWorktree,
  findPullRequest,
  GhError,
  GitActionError,
  git,
  githubRepoUrl,
  pushBranch,
} from '../src';

describe('workspace branch: commit, push, pull request', () => {
  let tmp: string;
  let repo: string;
  let origin: string;
  let ws: string;
  const savedPath = process.env.PATH;

  beforeEach(async () => {
    tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-branch-')));
    repo = join(tmp, 'repo');
    origin = join(tmp, 'origin.git');
    ws = join(tmp, 'ws');
    mkdirSync(repo);
    await git(repo, ['init', '-q', '-b', 'main']);
    await git(repo, ['config', 'user.name', 't']);
    await git(repo, ['config', 'user.email', 't@t']);
    writeFileSync(join(repo, 'README.md'), '# demo\n');
    await git(repo, ['add', '-A']);
    await git(repo, ['commit', '-qm', 'init']);
    await git(tmp, ['init', '-q', '--bare', origin]);
    await git(repo, ['remote', 'add', 'origin', origin]);
    await git(repo, ['push', '-q', 'origin', 'main']);
    await createWorktree({ repo, path: ws, branch: 'studio/ws/login', base: 'main' });
  });
  afterEach(() => {
    process.env.PATH = savedPath;
    rmSync(tmp, { recursive: true, force: true });
  });

  it('commits everything with the developer message, then pushes with an upstream', async () => {
    expect(await branchState(ws, 'origin/main')).toEqual({
      branch: 'studio/ws/login',
      uncommitted: 0,
      commits: 0,
      upstream: null,
      unpushed: null,
    });

    writeFileSync(join(ws, 'README.md'), '# changed\n');
    writeFileSync(join(ws, 'new.txt'), 'n\n');
    expect((await branchState(ws, 'origin/main')).uncommitted).toBe(2);
    await expect(commitWorkspace(ws, '  ')).rejects.toMatchObject({ code: 'empty_message' });

    const sha = await commitWorkspace(ws, 'Fix login\n\nDetails here');
    expect(await git(ws, ['log', '-1', '--format=%B'])).toBe('Fix login\n\nDetails here');
    expect(await git(ws, ['rev-parse', 'HEAD'])).toBe(sha);
    await expect(commitWorkspace(ws, 'again')).rejects.toMatchObject({ code: 'nothing_to_commit' });
    expect(await branchState(ws, 'origin/main')).toMatchObject({
      uncommitted: 0,
      commits: 1,
      upstream: null,
    });

    await pushBranch(ws);
    expect(await git(origin, ['rev-parse', 'studio/ws/login'])).toBe(sha);
    expect(await branchState(ws, 'origin/main')).toMatchObject({
      upstream: 'origin/studio/ws/login',
      unpushed: 0,
    });

    writeFileSync(join(ws, 'more.txt'), 'm\n');
    await commitWorkspace(ws, 'More');
    expect((await branchState(ws, 'origin/main')).unpushed).toBe(1);
    await pushBranch(ws);
    expect((await branchState(ws, 'origin/main')).unpushed).toBe(0);
    // The user's main is not touched.
    expect(await git(origin, ['rev-parse', 'main'])).toBe(await git(repo, ['rev-parse', 'main']));
  });

  it('runs the repository hooks on a commit and reports their output', async () => {
    const hook = join(repo, '.git', 'hooks', 'pre-commit');
    writeFileSync(hook, '#!/bin/sh\necho "lint failed: src/x.ts" >&2\nexit 1\n');
    chmodSync(hook, 0o755);
    writeFileSync(join(ws, 'x.txt'), 'x\n');
    const error = await commitWorkspace(ws, 'Try').catch((e: unknown) => e);
    expect(error).toBeInstanceOf(GitActionError);
    expect(error).toMatchObject({ code: 'hook_failed' });
    expect((error as GitActionError).detail).toContain('lint failed: src/x.ts');
  });

  it('never pushes a branch that is not studio/*', async () => {
    await expect(pushBranch(repo)).rejects.toMatchObject({ code: 'not_studio_branch', detail: 'main' });
  });

  it('knows the GitHub page of the repository', async () => {
    await git(repo, ['remote', 'set-url', 'origin', 'git@github.com:tolerance-dev/arena.git']);
    expect(await githubRepoUrl(repo)).toBe('https://github.com/tolerance-dev/arena');
    await git(repo, ['remote', 'set-url', 'origin', 'https://github.com/tolerance-dev/arena']);
    expect(await githubRepoUrl(repo)).toBe('https://github.com/tolerance-dev/arena');
    await git(repo, ['remote', 'set-url', 'origin', origin]);
    expect(await githubRepoUrl(repo)).toBeNull();
  });

  it('finds and opens pull requests through gh, and says when gh is missing or logged out', async () => {
    const bin = join(tmp, 'bin');
    mkdirSync(bin);
    const fakeGh = (script: string) => {
      writeFileSync(join(bin, 'gh'), `#!/bin/sh\n${script}\n`);
      chmodSync(join(bin, 'gh'), 0o755);
    };
    process.env.PATH = `${bin}:${savedPath}`;

    fakeGh('echo "$@" > "$(dirname "$0")/args"; echo \'[]\'');
    expect(await findPullRequest(ws, 'studio/ws/login')).toBeNull();
    fakeGh('echo \'[{"url":"https://github.com/o/r/pull/7"}]\'');
    expect(await findPullRequest(ws, 'studio/ws/login')).toBe('https://github.com/o/r/pull/7');

    fakeGh('echo "$@" > "$(dirname "$0")/args"; echo "https://github.com/o/r/pull/8"');
    expect(await createPullRequest(ws, { branch: 'studio/ws/login', base: 'main' })).toBe(
      'https://github.com/o/r/pull/8',
    );
    expect(readFileSync(join(bin, 'args'), 'utf8').trim()).toBe(
      'pr create --head studio/ws/login --base main --fill',
    );

    fakeGh('echo "To get started with GitHub CLI, please run:  gh auth login" >&2; exit 4');
    await expect(findPullRequest(ws, 'studio/ws/login')).rejects.toMatchObject({ code: 'logged_out' });

    process.env.PATH = '/nonexistent';
    const missing = await findPullRequest(ws, 'studio/ws/login').catch((e: unknown) => e);
    expect(missing).toBeInstanceOf(GhError);
    expect(missing).toMatchObject({ code: 'missing' });
  });
});
