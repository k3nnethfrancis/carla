# Policies, evaluations and training data

Carla separates judging text from what the application does with the judgment.
Monitoring, selection and evaluations have related criteria/specs but different
inputs, triggers and effects. Configuring one does not silently enable another.

| Role | Input / trigger | Result and effect |
| --- | --- | --- |
| Monitoring | Partial or completed generation and its context, at configured Heartbeat checkpoints | Local DiffusionGemma or hosted Jev behavior probabilities; enabled rules annotate, warn or explicitly stop. |
| Selection | Selection On, 2+ alternatives and explicit `--loops N` | Local instruct model reviews candidates and chooses one eligible continuation, or none. Others remain saved. |
| Evaluation | Frozen document/conversation items in a named collection | One LLM judge per policy records whole-item results for comparison and dataset curation. |

All are inspectable. A monitor flag is not automatically a training rejection;
a selected branch is not automatically an anthology or training member.

## Policy configuration

`/policy` has the same entry point in every tab: **Monitoring**, **Selection**,
and **Evals**. Each category holds named policies. A policy owns **Behaviors**,
**Judge** settings and the role’s result handling. Monitoring also has a
**Heartbeat** and Warn/Stop actions; Evals has **On pass**. Behaviors define what
to assess, the judge defines how to assess it, and the role determines how results
are used.

On the Monitoring or Selection policy list, **Space** or **Left/Right** toggles
the focused policy’s **On/Off** setting. **Enter** opens it. Turning a policy On
uses it for future runs and switches the previous policy in that category Off.
All policies can be Off. The built-in policy is named **Default policy**.
Inside the policy editor, **Status · Active/Inactive** is a read-only reflection
of On/Off. New policies start Off; editing their settings leaves them Off until enabled.

Evaluate → Policies is the same collection as `/policy` → Evals. Changes made
through either route affect the same policy. Data collections remain independent.

`/behaviors` manages reusable workspace specs (name and description). Import a
spec into a policy to copy its current revision, then configure the enabled state
and detection or passing rules for that use. Editing a library spec does not
silently change policies already using a copy, or any historical results.
Imports into Selection and Evals start Off. Review the wording and **Pass when**
before enabling them, especially specs originally written for a monitoring heartbeat.

## Monitoring

Open `/policy` → Monitoring → a policy. Default is Off. Simulator inherits the
saved switch. Document Continue/Loom starts with monitoring and selection Off;
use `--monitoring on` or `--selection on` to opt in for that run.
`--monitoring on|off` does not change saved settings. On uses the configured
provider, or the last explicitly selected provider if monitoring is currently Off.
If no provider has been configured, choose one in `/policy` first. DiffusionGemma (local) uses an
[OpenJev service](local-judge.md) without an API key. Selecting Jev requires a saved or
environment OpenRouter key before other settings appear. [Credential storage
and external data flow](configuration.md#optional-monitoring) apply here.

Open **Behaviors** directly inside the policy to name, describe and enable conditions.
Each behavior’s **Detection rule** sets when each probability counts as detected: Most
likely means above 50%; Threshold uses your cutoff. **Policy → Actions** sets
Warn/Stop and warning color for each behavior. Warn annotates the trace and
highlights detection; Stop ends the flagged generation/conversation. Disabled
conditions retain settings. Custom behaviors can be deleted; built-ins disabled.

**Heartbeat** separately enables checks during and after replies. **Interval**
sets the output-token interval for partial checks. Defaults when enabled are both
checks on and 512 tokens. Partial checks do not block token streaming, but pending
and final checks settle before the conversation advances. Errors remain visible
and fail open. Scores are model estimates, not calibrated guarantees.

## Selection

Open `/policy` → Selection → a policy → **Judge** to choose a registered local
instruct model, **Assessment call mode**, **Behavior assessment template**, and **Branch selection template**.
Selection behaviors have their own Name, multiline Behavior spec, Pass when,
library save/update state and Remove controls. Library updates preserve other
policy copies and past results.

Saving a changed template requires confirmation; cancelling preserves the draft.
[Template variables](templates.md) control where behaviors and candidate text appear.

Each enabled behavior is first assessed against each complete candidate. **Pass
when · Present/Absent** determines whether that observation is wanted. Separate
makes one assessment call per candidate and behavior; Bundled assesses all enabled
behaviors in one call per candidate. Only candidates passing every behavior reach
the final selection call, which uses the **Branch selection template** to compare them.
This adds assessment calls before selection. Both assessment and choice records
retain exact inputs, outputs and evidence. Failed assessments exclude that candidate;
if errors leave none eligible, the run reports failure rather than a criteria result.
The generator’s raw prompt never receives either judge template.

Selection triggers only when it is **On**, the Loom has **2 or more alternatives**,
and `--loops` is **explicitly supplied**. Bare `/loom`, `/continue`, and `/loom 4`
without `--loops` do not call the selector. `--loops 1` generates and judges one
batch, records a winner, and finishes. With Selection Off, loops continue all outputs.
Use `--selection on|off` to override the saved setting for one run. Explicit On
requires at least two alternatives and `--loops`; invalid combinations fail early.

With Selection On, `/loom 3 --tokens 512 --loops 4` creates three candidates per loop, classifies them
and advances one eligible path. The final chooser must review every eligible candidate with
`explore` or `pass`, a reason and matching evidence; `selected` identifies one
eligible candidate or null. Here `pass` means skip this candidate, not a passing
evaluation grade. No eligible candidate means no further expansion. On the last
loop, selection does not generate an extra continuation or automatically keep it.
The same loop mechanism operates on Simulator conversation extensions.

The generator unloads before local selection judging. Exact candidate context,
criteria, prompt and response are retained. Preserve the JSON contract if editing
the choice prompt; malformed responses remain errors.

## Data, Policies and Runs

Evaluate has three views and a run setup action:

- **Data** contains saved document versions and conversation traces. Collections
  organize those inputs; adding data does not call a model.
- **Policies** contains assessment definitions: a shared set of behaviors, judge
  settings and an On pass setting. Each judge owns its model and call settings and assesses
  the same enabled policy behaviors. A policy is independent of its input data.
- **Runs** contains the results of applying a policy to data. Each run preserves
  the input revisions and behavior configurations actually used.

Creating a dataset opens **Add** immediately, with Anthology documents first,
then other documents, conversations and existing judgments. Reopen **Add** to add
more items later. **Settings** lets you rename or remove the dataset. There is no
default dataset: direct `/eval` and `--eval` runs freeze their inputs in Runs without
adding them to a dataset. Add existing judgments to a dataset explicitly when wanted.
Use **+ New run** on the Evaluate page to configure one evaluation:

1. **Dataset:** choose saved data from Data.
2. **Policy:** choose an existing Evals policy (the same policies as `/policy` → Evals).
3. **Data:** assess all dataset items, or the subset selected before opening setup.
4. **On pass:** inherit the policy default, record only, or mark passing items for training.
5. **Start run:** execute and open Runs. Setup alone does not call a model.

You can also focus a dataset in Data and type `/eval`, or select items inside it
with Space and type `/eval`. Both open the same setup with that context filled in.
`/eval "Voice" --train-on-pass true` preselects a policy and run option. Changing
the dataset resets the item selection to all items in that dataset. Previously
evaluated items are included; every run records new results. Escape backs out of
pickers or cancels setup. These choices do not change the active policy.
Outside Evaluate, `/eval` still directly assesses selected documents/conversations.

To remove a collection, use **Settings → Remove collection**, or focus
it in the Data list and use `/remove` (`/delete` is an alias). Confirmation removes
the collection and its membership from Data. Source documents, conversations,
policies and historical results remain; a recovery copy is retained in the workspace.
Starting an evaluation opens its run in **Runs**. Running rows use an accent color.
The document pane shows the current item, judge and call count, streams actual LLM
judge text in a distinct color, and separates judge assessments from the saved input with headings and dividers. Each assessment identifies its judge and behavior. An incomplete run has no overall verdict: a PASS on one behavior does not override another assessment’s error. Classifier calls show
waiting progress followed by returned scores; they do not generate a text stream.

Open **Policies → a policy** to configure assessment:

- **Judge:** choose the policy’s LLM, call mode and assessment prompt
  template. Changed templates require confirmation before saving.
- **Behavior:** give it a name and spec, enable or disable it, and configure its
  **Pass when · Present/Absent** setting. The policy’s judge assesses it. For
  saved legacy classifier policies, the threshold applies to the probability of the desired outcome:
  with Absent selected, `P(absent) = 1 - P(present)`. The original observed
  probability is retained. LLMs report a boolean observation plus reason/evidence;
  Carla compares it with Pass when without inventing confidence probabilities.

**Separate** calls assess one enabled behavior per request. **Bundled** calls
assess the policy’s enabled behaviors together. Bundling reduces request count,
but can change the judgments; the UI asks you to confirm the tradeoff. Each
classifier behavior still has its own probability; scores are not normalized
against other behaviors. LLM judgments retain a boolean result, reason and evidence.

Set the active policy to choose what bare `/eval` runs. Monitoring and Selection
remain separately enabled operational policies. They each use one supported
judge, configured directly in **Judge** beside **Behaviors**. Evals uses the same
layout, with one LLM per policy and no heartbeat. The model picker currently
shows only registered LLMs. Existing classifier configurations and historical
results remain readable; no judge library or multiple-judge setup is offered.
Older multi-judge policies require an explicit single-model choice before running.
Their previous configurations are retained when replaced; frozen results stay intact.
Monitoring retains heartbeat and warn/stop actions. Selection uses the shared
whole-item assessment engine, then its own candidate-set choice contract.

Select data and run `/eval`, or choose a policy explicitly:

```text
/eval "Voice"
/eval "Voice" --train-on-pass true
/loom 4 --tokens 512 --eval "Voice"
/loom 3 --turns 2 --tokens 512 --eval "Voice" --loops 4
```

The name identifies a **policy**, not a data collection. The judge assesses all
enabled policy behaviors on the targeted items. Call mode determines whether
it assesses the behaviors separately or together.
Each local LLM judge resolves its own registered model. Model switches unload the
previous local LLM before loading the next; an unavailable model fails visibly.
Generation `--eval`
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
the data and rerun: pending-item coverage tracks saved judge and behavior configuration,
not changes to the file behind an alias. Execution traces retain the actual model
configuration. Editing source text
requires capturing the new version; it never rewrites a previously evaluated item.
Whole-item judging includes inherited document text or the saved conversation,
without shortening the input in Carla. Scope is explicitly the whole supplied
item for Evals and Selection, including when a reused spec mentions the latest
message. Review imported wording so it expresses the intended full-trace criterion.
Local LLMs check the rendered prompt against their context capacity before inference.
Classifier context enforcement belongs to their service; Carla preserves service
failures as incomplete assessments. LLM judge calls currently use temperature 0,
1536 output tokens and thinking disabled; these runtime defaults are not yet
per-judge controls.

Existing named evaluations migrate into data collections and policies. Their
saved judgments remain intact and available in run history. Older policies with
behaviors nested under judges are migrated to a shared behavior set. Identical
shared entries merge; conflicting versions are retained as separate entries.
Future runs apply the shared set through the policy’s single configured judge.
Legacy policies with multiple judges require an explicit model choice before
running again. Frozen historical runs keep their original assignments and results. Attaching existing
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
examples too. `/eval --train-on-pass true` marks only items whose enabled behaviors all pass
in that run; it does not undo an earlier manual training selection.

`/notes` records item notes. Notes/training changes have metadata histories.
The Evals policy’s **On pass** setting chooses **Record only** or **Mark for training**
by default. An explicit `--train-on-pass true|false` overrides that default for `/eval`.
Generation `--eval` uses the named policy’s saved default.

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
