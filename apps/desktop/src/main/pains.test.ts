import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { appendPain, painLine } from './pains';

describe('pains', () => {
  let home: string;
  beforeEach(() => {
    home = mkdtempSync(join(tmpdir(), 'studio-pains-'));
  });
  afterEach(() => rmSync(home, { recursive: true, force: true }));

  const at = new Date(2026, 9, 27, 14, 3);

  it('is one line with the time, project and workspace', () => {
    expect(painLine({ text: '  diff\n не обновился \t', project: 'arena', workspace: 'login', at })).toBe(
      '- 2026-10-27 14:03 · arena · login · diff не обновился',
    );
    expect(painLine({ text: 'нет проекта', project: null, workspace: null, at })).toBe(
      '- 2026-10-27 14:03 · — · — · нет проекта',
    );
  });

  it('appends to ~/.studio/pains.md, creating the folder and keeping what was there', () => {
    const studio = join(home, '.studio');
    const path = appendPain(studio, '- first');
    expect(path).toBe(join(studio, 'pains.md'));
    appendPain(studio, '- second');
    expect(readFileSync(path, 'utf8')).toBe('- first\n- second\n');

    writeFileSync(path, '# Мои заметки\n- edited by hand');
    appendPain(studio, '- third');
    expect(readFileSync(path, 'utf8')).toBe('# Мои заметки\n- edited by hand\n- third\n');
  });
});
