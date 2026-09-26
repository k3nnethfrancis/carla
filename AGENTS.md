# Carla contributor guidance

Read README.md and docs/architecture.md before changing the pipeline.

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
