# Studio

Desktop-приложение: сначала утверждаешь план изменения, потом агенты (Claude Code, Codex, OpenCode) параллельно реализуют его в git worktree, а Studio проверяет результат относительно плана.

Спецификация: [`docs/SPEC.md`](docs/SPEC.md). Ход работ: [`docs/milestones/`](docs/milestones/).

## Запуск

Нужны Node 22+ и pnpm.

```bash
pnpm install
pnpm dev      # окно приложения
pnpm check    # типы + линт + тесты
```
