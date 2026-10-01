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
       ├─ simulator + TurnMonitor: conversations and optional classification
       └─ evaluation + evaluation_sets: judges, frozen collections and exports
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
selection, simulation runs and anthology snapshots. `document_names.py` assigns
operation-first labels such as `doc-1`, `branch-2-loom-1-doc-1` and
`continue-1-branch-2-loom-1-doc-1`. These are display/provenance labels; immutable
node IDs remain reference keys. Parent-scoped counters survive deletion; explicit
user titles override labels. A shared operation ID groups Loom alternatives without
changing document ancestry or evaluation references.
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
in-memory data object. The snapshot/journal file layout is unchanged. Additive identity and grouping fields
are initialized lazily; older node IDs remain valid version references.
The store appends new chunks and provider events to `stream.jsonl`.
Full snapshots at turn/operation boundaries include a journal sequence and are
atomically renamed before the journal is removed. Recovery skips records at or
below that sequence; this prevents duplicate text if interrupted between those
steps. Writes are not fsynced, so this is process-crash recovery rather than a
power-loss durability guarantee. Full checkpoints remain synchronous and can
pause very large workspaces; per-token writes no longer serialize the workspace.

## Action identity and grouping

`document_actions.py` resolves document versions and cursor prefixes, then allocates
logical identities and ordered set membership. Node IDs remain immutable revision
references. `document_heads` points to current versions; `document_sets` records
frozen membership with logical set heads. Continue advances a head; older versions
and prefixes branch. `document_generation.py` orchestrates these plans through
Session's existing bounded streaming path. Unjudged loops advance every output;
explicit selection can choose whole alternatives between split loops.
Queued members are allocated before inference so cancellation retains set shape.

`simulator_actions.py` resolves explicit item/subset/set targets, validates visitor
message conflicts and records alternative groups. Continue archives the previous
run under `revisions` before advancing selected heads. Loom sets share an
`alternative_group` and retain ordered member runs. Generation uses one scheduler
across all sets; groups are domain structure, not extra model processes. Frozen
seeds record parent revision numbers, so moving a live head does not erase ancestry.

The frontend action contract exposes a reduced palette and common target plans.
Go does not create persistent IDs or implement selection policy. Backend validation
rejects unsupported scopes; execution cannot depend on a UI-only promise.

`exports.py` resolves saved selections and writes versioned JSON manifests with
text reading copies. It preserves structured turns, exact document ancestry and
judgment metadata without choosing training masks or a training framework. Export
is independent of the live snapshot/journal persistence and is not workspace restore.

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

`stream_monitor.py` snapshots character prefixes for optional local OpenJev or hosted OpenRouter scans.
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

The optional DiffusionGemma classifier runs in a Carla-owned OpenJev MLX process.
`local_judge.py` starts its cached runtime on a kernel-assigned loopback port, shares
it across checks, and stops it when its owning backend disconnects. It is separate
from the generator admission/swap system;
its memory and GPU work coexist with llama.cpp. Local HTTP endpoints are loopback
only, with redirects/proxy inheritance disabled, no credentials and no remote
fallback. The pinned launcher disables OpenJev routing and loads cached weights
offline after setup. See [local judge setup](local-judge.md).

## Evaluation data, policies and runs

`evaluation_judges.py` owns the Policy → Judges → Behaviors schema, validation
and legacy definition conversion. `evaluation.py` owns assessment execution.
Local LLM assessment reuses `Runtime.judge`; DiffusionGemma/Jev assessment shares
`monitor.classify`, retaining exact requests and provider results. Assessment
prompts never enter generation context. Monitoring and selection retain their
operational owners and context-specific actions.

`operational_policies.py` stores named Monitoring and Selection configurations,
active choices, and the workspace behavior-spec library. It adapts a selected
policy to the existing generation runtime settings. Editing an inactive policy
does not activate it; legacy configuration commands update the active policy.
The UI uses one `/policy` hub in every tab. Evaluate Policies is another entry
to the same Evals catalog. Library imports copy the spec and its revision;
changing a library entry does not mutate existing uses or historical runs.

`evaluation_sets.py` owns data collections, frozen item membership, evidence
references and training metadata. `evaluation_policies.py` owns reusable policy
groups and run envelopes. The workspace stores:

- `evaluators`: retained legacy definitions for compatibility and migration.
- `evaluation_sets`: data collections and frozen items.
- `behavior_library`: reusable, revisioned names and specs.
- `operational_policies` and `active_operational_policies`: named Monitoring and
  Selection configuration and explicit activation per category.
- `evaluation_policies`: policy actions and judges (model, prompt, call mode) containing
  versioned behavior specs, enabled states and detection thresholds.
- `evaluations`: individual assessment records with frozen inputs and definitions.
- `evaluation_runs`: execution envelopes referencing those records and preserving
  the policy used. Historical run status comes from its records, not current policy.

The additive migration creates policies from legacy collection configurations
and run envelopes from historical result batches. It does not rewrite original
judgments. Adding snapshots or attaching completed judgments invokes no model.
Policy evidence retains turn/candidate scope rather than becoming a whole-item grade.

`/eval [policy]` captures targets before a cancellable session job. Generation
`--eval` freezes the chosen policy, judges and behaviors at dispatch and chains
assessment after generation under the same operation lock. Reruns append results.
Item notes and training membership have metadata histories. Snapshot events carry
data, policy and run summaries; opening an item or run requests full results.
Exports preserve frozen data, judgment history, notes and training marks in a
workspace-local structured manifest plus text copies; they do not train a model.

The Evaluate TUI uses the existing focus, editor, selection and Escape mechanisms
for Data, Policies and Runs. Data collection membership and policy selection are
independent; no additional named evaluation container is required.

The frontend saves the last tab and document/trace row in each workspace's
`view-state.json`, separately from project data. Startup restores that location
with keyboard focus in the command bar. It does not restore checked Loom targets,
editing, dialogs, or command input. Missing/deleted rows fall back to the tab's
first item. Navigation writes are atomic and occur only when the location changes.

### Action scope

`action_scope.py` defines recursive containment (`document` or `conversation`
leaves and `set` children). It is separate from parent/version ancestry.
Go sends the explicit selected shape; Python validates/freeze-copies it, enumerates
leaves for scheduling, and remaps saved output scopes to new IDs on splits.
Continue retains conversation identities; document continuations save child
versions. Current document sets resolve their current member heads, while old
set snapshots and evaluated versions remain unchanged.
