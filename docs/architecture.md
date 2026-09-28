# Architecture

```text
Go / Bubble Tea + Lip Gloss
  focus, layout, editing buffers, command completion, grids
                │ commands / events (localhost NDJSON)
Python / asyncio Session
  validation, workspace ownership, one active operation
       ├─ Project: document changes and curation
       │     ├─ ancestry: pure text-origin and first-change calculations
       │     └─ WorkspaceStore: snapshots, stream journal and recovery
       ├─ library: shared source documents
       ├─ Runtime + Admission: local llama.cpp, bounded requests
       ├─ exploration: shared generate → unload → classify → advance loops
       └─ simulator + TurnMonitor: conversations and optional classification
```

## Boundaries and protocol

Go starts one Python child. Python binds `127.0.0.1:0` and sends `{v, port, token}`
on its private stdout pipe. The client authenticates with that per-launch token.
Subsequent frames are UTF-8 newline-delimited JSON. Protocol version is 1.
This is an internal local protocol, not a stable public API.

```json
{"v":1,"id":"12","command":"continue","args":{"branch":true,"offset":340}}
{"v":1,"seq":41,"type":"token","id":"12","data":{"node":"branch-id","text":" A path"}}
```

Events have a monotonic connection sequence. Replies to short mutations carry
the request ID; background updates may have no request ID. The editor retains
its draft until the state reply for its own save arrives.
A single writer lock and awaited socket drain preserve ordering and apply
backpressure. Text offsets are Unicode code points, not bytes or terminal cells.
Python emits data, not ANSI or layout instructions. Go never writes workspace JSON.

Short mutations run serially. Long operations run in a cancellable task, with
up to four workers submitting requests to one resident model. Workspace switching
and incompatible edits are rejected while generation is active. Disconnect
cancels generation and saves partial output. A forced kill recovers from the
checkpoint and journal on next open; interrupted output is labeled accordingly.

## Persistence

`domain.py` owns source, generated and edited document lineage, annotations,
selection, simulation runs and anthology snapshots. Document versions receive stable
workspace-wide identifiers such as `paths-branch-0001`, `paths-gen-0001`,
and `paths-edit-0001`. Nested tree rows omit the source prefix; headings and
Anthology retain it. Counters survive deletion; explicit user titles override labels.
Existing workspaces receive labels on their next save without changing text or lineage.
Source text and generation
prompts are copied into the artifacts so later library edits cannot rewrite history.
Conversation edits fork through the changed turn and discard later replies only
in the fork. Curation and export do not trigger training.

`ancestry.py` computes inherited source/AI/human spans, remaps them across edits,
and finds a version's first change. It receives a node index and performs no I/O
or mutation. `Project` owns document changes and builds that index when needed;
selection, curation and export remain document operations rather than new layers.

`persistence.WorkspaceStore` loads the JSON snapshot, replays its stream journal,
and owns the write → atomic replace → journal compaction sequence. `Project`
retains interrupted-status decisions and document labels; both share the same
in-memory data object. The existing file layout and schema are unchanged.
The store appends new chunks and provider events to `stream.jsonl`.
Full snapshots at turn/operation boundaries include a journal sequence and are
atomically renamed before the journal is removed. Recovery skips records at or
below that sequence; this prevents duplicate text if interrupted between those
steps. Writes are not fsynced, so this is process-crash recovery rather than a
power-loss durability guarantee. Full checkpoints remain synchronous and can
pause very large workspaces; per-token writes no longer serialize the workspace.

## Inference and scheduling

`runtime.py` uses raw `/completion` with exact prompt text. `scheduling.py`
reserves the full prompt plus output budget against the server's shared context,
and checks slot limits and available host memory. It never reduces a budget to
increase parallelism. Host memory is only a heuristic; discrete GPU VRAM and
optimal throughput are not modeled. `Max` commonly serializes requests.

Both raw generation and policy selection resolve capacity from the loaded server's
`/props`; configured `0` means native context. Neither truncates a prompt to fit.
Raw streams require a terminal `stop: true` event or `[DONE]` marker. EOF without
one fails the operation, preserving partial text and provider events. Trace stream
status distinguishes completion, interruption, provider failure and cancellation.

`simulator_commands.py` interprets Simulator configuration, monitor policy,
conversation view/fork and run commands. `Session` keeps workspace locking and
active-job ownership; `simulator.py` owns the actual conversation generation.

`simulator.py` groups adjacent speaker roles by model alias. Within one segment,
each conversation advances independently, including its own monitor wait.
Model changes are barriers: finish the current segment, unload its model, then
load the next. Same weights configured under different aliases still count as
different models. The server performs continuous batching, not separate GPUs.

Simulation prompts contain the frozen anthology plus transcript for the character,
and visitor brief plus transcript for the visitor. Templates are configurable.
Each turn records model, settings, prompt, response events and observations.
The policy selector has a separate instruct-model chat request; its specification
is never added to the character context.

`stream_monitor.py` snapshots character prefixes for optional OpenRouter scans.
It permits one in-flight check per conversation, coalesces additional tokens,
and keeps exact request/response evidence. Final checks are awaited before that
conversation advances; other conversations can proceed. Explicit Stop actions
signal only the affected conversation. Each provider read races that signal;
cancellation drains the pending read and closes the stream before releasing its
capacity reservation. Warn and monitor-provider errors do not stop generation.

## Frontend

`tui/` owns command routing, contextual completion, keybindings, tree selection,
conversation grids and the document editor. Library, Branches, Anthology and
Simulator are the main views. Notes belong to documents. The terminal supplies
light/dark base colors; provenance and speaker roles use distinct accents.
Narrow terminals collapse panels; below 60 × 18 only a resize/quit view is shown.

See [commands](commands.md) for the command contract. `exploration.py` owns repeated
batches and evidence-backed selection; document and conversation generators own
their outputs. `stream_monitor.py` shares bounded monitoring across both.

See `service.py` for backend commands, `tui/command.go` for command descriptions,
and tests alongside each subsystem for its executable behavioral contract.

## Model onboarding

Setup uses the existing Go dialog stack (rows, inputs, parent/back navigation and
semantic colors). The backend emits structured `setup` events for catalog choices,
resolved download plans, progress, completion and errors. Selecting a source fetches
metadata only; a separate command confirms the download. Local imports do not copy
weights. The transfer runs in a child process so cancellation stops the Hub's
worker threads and leaves its partial cache reusable. Registration and model
selection happen in the owning Session only after a successful transfer.

## Evaluation records

`evaluation.py` owns versioned judge definitions and execution. Local judging
reuses `Runtime.judge`; Jev evaluation and monitoring share `monitor.classify`,
retaining exact requests and provider results. Evaluation prompts never enter
generation context. Monitoring and selection retain their operational owners.

`evaluation_sets.py` owns named collections, frozen item membership, evidence
references and training metadata. Workspace `evaluators` holds current judge
definitions; `evaluations` holds immutable completed judgment records;
`evaluation_sets` holds collections and `active_evaluation` selects the default.
A one-time additive migration references historical results without rewriting them.
Adding snapshots or attaching completed judgments requires no model call. Policy
evidence retains turn/candidate scope rather than becoming a whole-item grade.

`/eval` captures targets before a cancellable session job. `/loom --eval name`
freezes judge configuration at dispatch and chains evaluation after generation
under the same operation lock. Re-evaluation appends results. Item notes and
training membership have metadata histories. Normal state events carry collection
summaries; opening an item requests its full text/evidence separately. Export
writes training-marked items to a new workspace-local JSONL; it does not train.

`tui/evaluation_collections.go` owns collection navigation, membership/configuration
dialogs and item rendering. `tui/evaluation.go` owns judge dialogs and command
execution. Both reuse the app's focus, editor and parent/back mechanisms.

The frontend saves the last tab and document/trace row in each workspace's
`view-state.json`, separately from project data. Startup restores that location
with keyboard focus in the command bar. It does not restore checked Loom targets,
editing, dialogs, or command input. Missing/deleted rows fall back to the tab's
first item. Navigation writes are atomic and occur only when the location changes.
