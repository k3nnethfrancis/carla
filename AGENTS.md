# Carla contributor guidance

Read [README.md](README.md) for setup, [architecture](docs/architecture.md) for
ownership, and [development](docs/development.md) for issues, PRs and releases.
For UI work, use only the relevant guidance in
[skills/tui-design/SKILL.md](skills/tui-design/SKILL.md), Carla's own adaptation.

- `src/character_lab/` owns the backend; `tests/` its regressions.
- `tui/` owns the frontend and Go interaction tests.
- `.github/` and `scripts/` own contribution, build and release operations.

Use GitHub Issues for actionable bugs/features and a focused branch/PR for changes.
Keep research notes and experiments local; don't create a parallel repo task ledger.
Before handing off, run `make test lint build`, exercise changed terminal journeys,
and report the outcome, evidence and remaining gaps. Don't publish releases or
change repository visibility merely because checks pass.

- Python owns domain state, prompts, inference, persistence and job lifecycles.
  Go owns terminal interaction and rendering. Exchange structured NDJSON events.
- Base-model generation is raw completion. Preserve exact source text, prompts,
  settings, model identity, branches, edits, partial output and failures.
- Selection policies are separate from generation. Never inject hidden assistant
  instructions, memory, reflection or character objectives into base prompts.
- One resident managed model at a time. Concurrent requests may share its server;
  never shrink context/output budgets to admit more work.
- Monitoring is optional. Only explicitly configured Stop actions stop a flagged
  conversation. Token caps and quality observations do not stop later turns.
- Keep workspaces, downloaded texts, weights, credentials and experiments outside
  source control. Use synthetic fixtures and explicit portable configuration.
- Keep changes small and documented at the relevant boundary. Run meaningful
  Python/Go tests and verify terminal interactions for UI changes.
- This is an experimental curation tool. Training is not implemented; describe
  method deviations honestly and avoid implying reproduction of published models.
