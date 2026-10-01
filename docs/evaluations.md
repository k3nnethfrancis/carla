# Policies, evaluations and training data

Carla separates judging text from what the application does with the judgment.
Monitoring, selection and evaluations have related criteria/specs but different
inputs, triggers and effects. Configuring one does not silently enable another.

| Role | Input / trigger | Result and effect |
| --- | --- | --- |
| Monitoring | Partial or completed generation and its context, at configured Heartbeat checkpoints | Local DiffusionGemma or hosted Jev behavior probabilities; enabled rules annotate, warn or explicitly stop. |
| Selection | Selection On, 2+ alternatives and explicit `--loops N` | Local instruct model reviews candidates and chooses one eligible continuation, or none. Others remain saved. |
| Evaluation | Frozen document/conversation items in a named collection | One or more instruct/DiffusionGemma/Jev judges record whole-item results for comparison and dataset curation. |

All are inspectable. A monitor flag is not automatically a training rejection;
a selected branch is not automatically an anthology or training member.

## Monitoring

Open `/policy` → Monitoring. Default is Off. `--monitoring on|off` overrides it
for one Continue/Loom run without changing saved settings. On uses the configured
provider, or the last explicitly selected provider if monitoring is currently Off.
If no provider has been configured, choose one in `/policy` first. DiffusionGemma (local) uses an
[OpenJev service](local-judge.md) without an API key. Selecting Jev requires a saved or
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

Selection triggers only when it is **On**, the Loom has **2 or more alternatives**,
and `--loops` is **explicitly supplied**. Bare `/loom`, `/continue`, and `/loom 4`
without `--loops` do not call the selector. `--loops 1` generates and judges one
batch, records a winner, and finishes. With Selection Off, loops continue all outputs.
Use `--selection on|off` to override the saved setting for one run. Explicit On
requires at least two alternatives and `--loops`; invalid combinations fail early.

With Selection On, `/loom 3 --tokens 512 --loops 4` creates three candidates per loop, classifies them
and advances one eligible path. The classifier must review every candidate with
`explore` or `pass`, a reason and matching evidence; `selected` identifies one
eligible candidate or null. Here `pass` means skip this candidate, not a passing
evaluation grade. No eligible candidate means no further expansion. On the last
loop, selection does not generate an extra continuation or automatically keep it.
The same loop mechanism operates on Simulator conversation extensions.

The generator unloads before local selection judging. Exact candidate context,
criteria, prompt and response are retained. Preserve the JSON contract if editing
the classifier prompt; malformed responses remain errors.

## Data, Policies and Runs

Evaluate has three views:

- **Data** contains saved document versions and conversation traces. Collections
  organize those inputs; adding data does not call a model.
- **Policies** contains named groups of behaviors, each with its specification,
  model and passing rule. A policy is independent of the data it assesses.
- **Runs** contains the results of applying a policy to data. Each run preserves
  the input revisions and behavior configurations actually used.

Create or open a data collection and use **Add data** to add saved documents,
conversations or existing judgments. **Collection settings** lets you rename the
collection or make it the default destination for data assessed from other tabs. In Policies, create a policy and choose its behaviors. A
behavior uses a local LLM prompt or a DiffusionGemma/Jev classifier spec and
probability threshold. Set the active policy to choose what bare `/eval` runs.
Monitoring and Selection remain separately enabled operational policies; creating
an evaluation policy does not turn either on.

Select data and run `/eval`, or choose a policy explicitly:

```text
/eval "Voice"
/eval "Voice" --train-on-pass true
/loom 4 --tokens 512 --eval "Voice"
/loom 3 --turns 2 --tokens 512 --eval "Voice" --loops 4
```

The name now identifies a **policy**, not a data collection. All behaviors in
that policy assess the targeted items, currently sequentially. Generation `--eval`
freezes the chosen configuration before generation and evaluates completed outputs
without altering generation prompts. Cancellation does not start an evaluation
phase. Generation and evaluation share the session operation lock.

## Interpret and preserve results

Open Runs to inspect an execution and its judgments. Every assessment retains
its input text, source lineage, generation-model provenance, behavior revision,
request/response and result. A passing item requires every assigned behavior to
complete and pass. Errors and interruptions are incomplete assessments, not
failed criteria. Classifier probabilities are estimates, not calibrated certainty;
LLM boolean judgments do not invent probabilities.

Changing a policy or behavior affects future runs. Historical runs retain their
original configuration and results. Rerunning appends results. If you replace a local LLM behind the same configured alias, explicitly select
the data and rerun: pending-item coverage currently tracks behavior revisions,
not changes to the file behind an alias. Execution traces retain the actual model
configuration. Editing source text
requires capturing the new version; it never rewrites a previously evaluated item.
Whole-item judging includes inherited document text or the saved conversation,
without silent truncation. Inputs exceeding model context fail visibly.

Existing named evaluations migrate into data collections and policies. Their
saved judgments remain intact and available in run history. Attaching existing
monitoring observations preserves their partial/turn scope; selection evidence
preserves its candidate-set context. Neither becomes a whole-item grade merely
because it was added to Data.

Use the same data and policy to assess prompt, sampling or model changes as well
as training changes. Generate variants separately and record what changed. Carla
preserves evidence; it does not yet orchestrate a controlled benchmark matrix,
calibrate judges or establish causal claims automatically.

## Build and export a training set

The training controls mark or unmark selected evaluation items. `/remove` removes
items from the collection; stored judgments and a recovery record are preserved.
The compatibility command `/keep` remains available for this action.
Manual selection is independent of the judge result, allowing deliberate negative
examples too. `/eval --train-on-pass true` marks only items whose judges all pass
in that run; it does not undo an earlier manual training selection.

`/notes` records item notes. Notes/training changes have metadata histories.
`/export` saves explicitly selected items, or the entire open collection, under
the workspace's `exports/` directory. It writes a versioned `manifest.json` plus
text reading copies. Training marks are preserved; they do not silently filter
the export. Each structured item includes:

- Frozen text, source records, generation-model provenance and snapshot hash.
- Collection identity, judgment history and attached scoped policy evidence.
- Training membership, notes and metadata history.

Keep exports as reproducible artifacts. Downstream training preparation must
choose train/validation splits, deduplicate related lineages, assign speaker loss
masks and construct any preference pairs deliberately. Carla does not currently
perform those steps or launch SFT, KTO, DPO or RL training. A judge label alone
is not a validated reward signal or proof of character quality.
