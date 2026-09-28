# Policies, evaluations and training data

Carla separates judging text from what the application does with the judgment.
Monitoring, selection and evaluations have related criteria/specs but different
inputs, triggers and effects. Configuring one does not silently enable another.

| Role | Input / trigger | Result and effect |
| --- | --- | --- |
| Monitoring | Partial or completed generation and its context, at configured Heartbeat checkpoints | Jev behavior probabilities; enabled rules annotate, warn or explicitly stop. |
| Selection | Candidate set during a multi-loop Loom | Local instruct model reviews candidates and chooses one eligible continuation, or none. Others remain saved. |
| Evaluation | Frozen document/conversation items in a named collection | One or more local/Jev judges record whole-item results for comparison and dataset curation. |

All are inspectable. A monitor flag is not automatically a training rejection;
a selected branch is not automatically an anthology or training member.

## Monitoring

Open `/policy` → Monitoring. Default is Off. Selecting Jev requires a saved or
environment OpenRouter key before other settings appear. [Credential storage
and external data flow](configuration.md#optional-monitoring) apply here.

In **Behaviors**, each condition has a name, spec, enabled status, detection rule
and action. Most likely means estimated probability above 50%; Threshold uses
your chosen cutoff. Warn annotates the trace and highlights detection; Stop ends
the flagged generation/conversation. Disabled conditions retain their settings.
Custom behaviors can be deleted; built-in ones can be disabled.

**Heartbeat** separately enables checks during and after replies. **Interval**
sets the output-token interval for partial checks. Defaults when enabled are both
checks on and 512 tokens. Partial checks do not block token streaming, but pending
and final checks settle before the conversation advances. Errors remain visible
and fail open. Scores are model estimates, not calibrated guarantees.

## Selection

Open `/policy` → Selection to configure the local instruct evaluator, criteria
and classifier/routing prompt. The generator's raw prompt never receives these
instructions. The selector receives candidate text and criteria separately.

`/loom 3 --tokens 512 --loops 4` creates three candidates per loop, classifies them
and advances one eligible path. The classifier must review every candidate with
`explore` or `pass`, a reason and matching evidence; `selected` identifies one
eligible candidate or null. Here `pass` means skip this candidate, not a passing
evaluation grade. No eligible candidate means no further expansion. On the last
loop, selection does not generate an extra continuation or automatically keep it.
The same loop mechanism operates on Simulator conversation extensions.

The generator unloads before local selection judging. Exact candidate context,
criteria, prompt and response are retained. Preserve the JSON contract if editing
the classifier prompt; malformed responses remain errors.

## Create and run an evaluation

1. Open **Evaluate** → **New evaluation** and name it, for example `Voice`.
2. Open **Configure** (`/config`). Choose reusable judges. If none exist, create
   one under **Manage judge configurations** or `/policy` → **Judge configurations**.
   A local judge has criteria and an editable system prompt. A Jev judge uses a
   spec and pass-probability threshold. Local judges currently use the configured
   instruct policy-model alias, not an arbitrary independent model per judge.
3. Set **Active evaluation** to choose the default used by `/eval` outside this tab.
4. **Add items / existing judgments** selects saved document versions, conversations
   or completed judgment records. Adding captures text/provenance and available
   evidence; it does not invoke a model.
5. **Run selected / pending items** or `/eval` executes judges. Checked items take
   priority, otherwise a highlighted item is rerun; from an action row, pending
   items run. Enter opens an item and its judgment history. **Show** filters results.

From Branches/Anthology, `/eval` targets checked versions or the highlighted one.
From Simulator it uses an explicitly selected individual conversation, not a
hovered row or an entire batch. `/eval "Voice"` chooses a named collection.
Inside an open evaluation, bare `/eval` uses that collection.

```text
/eval "Voice"
/eval "Voice" --train-on-pass true
/loom 4 --tokens 512 --eval "Voice"
/loom 3 --turns 2 --tokens 512 --eval "Voice" --loops 4
```

`--eval` freezes the chosen judge configuration when the run starts and judges
completed outputs after generation. It does not alter model prompts or replace
selection. Cancellation does not start a new evaluation phase. Generation and
judging share the session operation lock; judges currently run sequentially.

## Interpret and preserve results

Each item stores frozen text, source lineage and generation-model provenance.
Each judgment preserves its judge revision, request/response, reason and evidence.
A collection-level pass requires every currently configured judge revision to
complete and pass. Errors/interruption are incomplete, not failed criteria.
Jev probabilities use your cutoff; they are not validated confidence estimates.

Editing a judge creates a new revision. Old results remain readable but do not
count as passes under the revised criteria. Rerunning appends judgment history.
Editing source text requires adding the new version; it never rewrites an old item.
Whole-item judging includes inherited document text or the saved conversation,
without silent truncation. Inputs exceeding model context fail visibly.

You can attach previously completed judgments without rerunning them. Attached
monitoring observations retain their partial/turn scope; selection evidence retains
its candidate-set context. Neither is relabeled as a whole-item grade. Historical
flat results migrate additively into collections on workspace open.

These collections support evaluations of prompt, sampling or model changes as
well as future training changes. Generate each variant separately and record what
changed. Carla preserves evidence; it does not yet run a controlled benchmark
matrix, calibrate your judges or establish causal claims automatically.

## Build and export a training set

`/keep` marks selected evaluation items for training. `/remove` unmarks them.
Manual selection is independent of the judge result, allowing deliberate negative
examples too. `/eval --train-on-pass true` marks only items whose judges all pass
in that run; it does not undo an earlier manual training selection.

`/notes` records item notes. Notes/training changes have metadata histories.
`/snapshot` exports the open collection's training-marked items to a new JSONL file
under the workspace's `datasets/` directory. Each line includes:

- Frozen text, source records, generation-model provenance and snapshot hash.
- Collection identity, judgment history and attached scoped policy evidence.
- Training membership, notes and metadata history.

Keep exports as reproducible artifacts. Downstream training preparation must
choose train/validation splits, deduplicate related lineages, assign speaker loss
masks and construct any preference pairs deliberately. Carla does not currently
perform those steps or launch SFT, KTO, DPO or RL training. A judge label alone
is not a validated reward signal or proof of character quality.
