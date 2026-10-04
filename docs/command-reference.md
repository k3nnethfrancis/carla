# Command reference

Generated from `tui/command_registry.go`, the metadata used by help and completion.
For a first experiment, start with the [README walkthrough](../README.md#from-a-seed-to-evaluated-conversations).
Availability depends on the current stage and target. Compatibility commands remain searchable.

## /active

Jump to the active generation while leaving background work running.

## /add

Library: import a source file. Branches: add selected versions to Anthology. Anthology: choose existing versions. Evaluate: add saved traces without judging.  
  
ALIASES  
/import · /keep

In Evaluate, aliases: /add · /import.

## /behaviors

Create reusable behavior specs. Import a copy into a policy; detection rules stay with its behaviors and responses with the policy.

## /branch

Copy selected documents, conversations or sets without generation, preserving ancestry. Anthology opens the copies in Branches. Unavailable in Library and Evaluate. Save edits first.  
  
ALIASES  
/fork

## /branches

Browse continuations; LEFT collapses, RIGHT expands, ENTER opens.

## /stop

Stop the active generation; preserve its partial output.

## /clear

Clear source/branch selection. In Simulator clear the conversation target so the next Loom starts fresh. Does not delete saved content.

## /config

Configure generation models, prompts, temperature, top-p and sampling; in Evaluate, configure the opened dataset. /policy owns monitoring, selection and Evals judges and behaviors. Loom flags override generation settings for one run.  
  
SYNTAX  
/config [setting] e.g. /config turns  
  
DEFAULTS  
Branches: Tokens, model, sampling and context. Simulator: Turns, Character tokens, Visitor tokens, speaker models and prompts. Flags override one run; /config saves defaults.  
  
/config keys  
/config model  
/config workspace  
  
ALIASES  
/configure · /settings · /sim-config · /character-sampling · /visitor-sampling

## /continue

Advance selected items. Documents save a revision; conversations advance their saved histories.  
  
/continue [flags]  
  
GENERATION FLAGS  
--tokens N|Max  
  Cap new tokens per completion; not context size.  
--loops N  
  Repeat the operation. Omitted: one pass.  
--model alias  
  Override the document or character model.  
  
SIMULATOR FLAGS  
--turns N  
  Additional character replies per loop.  
--visitor "text"  
  Supply the next visitor message once, to each target.  
--visitor-model alias  
  Override the visitor model.  
  
POLICY FLAGS  
--monitoring on|off  
  Enable or bypass the configured monitor this run.  
--eval "policy name"  
  Assess completed outputs with a named Evals policy.  
--selection off  
  Bypass selection. Continue cannot enable it.  
  
DEFAULTS & SCOPE  
/config saves tokens, turns and model defaults. Flags affect only this run. Documents default to judging Off; Simulator inherits saved policy switches. --tokens controls output; context includes input plus output.  
  
Branches: continue or split selected documents. Anthology: /continue opens a new Branches continuation; /loom starts Simulator from kept documents. New versions are not automatically kept.  
  
Simulator: select a parent for the whole set, or children for a subset. Arrows only preview. /clear makes the next Loom fresh. /continue requires a target. Library and Evaluate do not generate.  
  
EXAMPLES  
/continue --tokens 512  
/loom 4 --tokens 512  
/loom 2 --tokens 512 --turns 3 --loops 2  
/continue --visitor "Why?" --turns 1  
/loom 4 --selection on --loops 1  
/loom 2 --eval "Voice" --monitoring off  
  
The --turns and --visitor examples require Simulator (or Anthology Loom). Here --turns 3 --loops 2 adds six character replies per conversation. With selection Off, loops advance every output; with it On, later loops split again from the winning alternative.  
  
SPELLINGS  
Flags accept --flag=value and any order; examples put --loops last. --msg and --message mean --visitor.  
  
ALIASES  
/generate · /run · /simulate

## /cancel

Cancel the draft and return to document navigation; also ESC.

## /edit

Edit existing document text or a conversation message; saving preserves the original as a new version.

## /eval

Run /eval [policy] on selected document versions or checked conversations. In Evaluate, open New run setup with the focused dataset or selected items. Uses the active policy when no name is given. --train-on-pass true marks passing results for training; the policy default applies when omitted.  
  
SYNTAX  
/eval ["policy name"] --train-on-pass true|false  
  
EXAMPLES  
/eval  
/eval "Voice" --train-on-pass true  
  
TARGET  
Branches/Simulator: saved selected items. Evaluate: opens New run setup. No implicit default dataset.  
  
--train-on-pass true|false  
Override the policy's training-mark default for this run. Every enabled behavior must complete and pass.

## /evaluate

Organize datasets in Data, open Evals policies in Policies, inspect Runs, and configure an evaluation with + New run. /export saves selected items or the opened collection with metadata.  
  
ALIASES  
/evaluations

## /exit

Exit Carla; preserves unsaved drafts in recovery files.  
  
ALIASES  
/quit

## /find

Filter documents or runs on the current page; type in the focused filter.

## /grid

Show concurrent Loom outputs. Arrows select a tile; Enter opens it. /grid returns.

## /help

Show all commands and keyboard navigation.

## /import

Add a local UTF-8 .txt or .md document to the shared Library. Title defaults to filename; author and source URL are optional.

## /inspect

Read generation settings, monitoring findings, selection reasons and evaluation verdicts together in one scrollable report. Enter opens raw evidence; token events are separate. Escape returns. Missing historical evidence is labeled as not recorded. Retry selection asks for confirmation, uses frozen candidates and policy, saves a new attempt, and generates no text.

## /keep

Keep checked branches (or the highlighted branch) in Anthology. In Evaluate, mark selected items for training.

## /anthology

Browse the anthology of kept document versions.  
  
ALIASES  
/kept

## /keys

Assign keys to actions and navigation; bindings apply across workspaces.

## /library

Browse seed documents and select passages.

## /loom

Continue once, or split into alternative futures.  
  
/loom [N] [flags]  
With a target: N = 1 continues; N = 2+ splits.  
No Simulator target: start N fresh conversations.  
  
GENERATION FLAGS  
--tokens N|Max  
  Cap new tokens per completion; not context size.  
--loops N  
  Repeat the operation. Omitted: one pass.  
--model alias  
  Override the document or character model.  
  
SIMULATOR FLAGS  
--turns N  
  Additional character replies per loop.  
--visitor "text"  
  Supply the next visitor message once, to each target.  
--visitor-model alias  
  Override the visitor model.  
  
POLICY FLAGS  
--monitoring on|off  
  Enable or bypass the configured monitor this run.  
--eval "policy name"  
  Assess completed outputs with a named Evals policy.  
--selection on|off  
  Choose among alternatives. On requires N >= 2 and an explicit --loops N. --loops 1 selects once.  
  
DEFAULTS & SCOPE  
/config saves tokens, turns and model defaults. Flags affect only this run. Documents default to judging Off; Simulator inherits saved policy switches. --tokens controls output; context includes input plus output.  
  
Branches: continue or split selected documents. Anthology: /continue opens a new Branches continuation; /loom starts Simulator from kept documents. New versions are not automatically kept.  
  
Simulator: select a parent for the whole set, or children for a subset. Arrows only preview. /clear makes the next Loom fresh. /continue requires a target. Library and Evaluate do not generate.  
  
EXAMPLES  
/continue --tokens 512  
/loom 4 --tokens 512  
/loom 2 --tokens 512 --turns 3 --loops 2  
/continue --visitor "Why?" --turns 1  
/loom 4 --selection on --loops 1  
/loom 2 --eval "Voice" --monitoring off  
  
The --turns and --visitor examples require Simulator (or Anthology Loom). Here --turns 3 --loops 2 adds six character replies per conversation. With selection Off, loops advance every output; with it On, later loops split again from the winning alternative.  
  
SPELLINGS  
Flags accept --flag=value and any order; examples put --loops last. --msg and --message mean --visitor. --count N and -n N mean the positional count.  
  
SET EXAMPLE  
Four selected conversations + /loom 2 creates two alternative sets of four. /continue advances the original four. One output continues; two or more split alternatives.  
  
ALIASES  
/grow

## /model

Choose the Loom base model; on Simulator choose Character or Visitor. /model visitor jumps directly to that picker.  
  
SYNTAX  
/model [character|visitor]

## /notes

Open this document’s notes. + adds a note; Enter reads and jumps to its anchor.

## /policy

Configure Monitoring, Selection and Evals policies. Each policy has one judge and behavior specs. Monitoring checks during generation; Selection chooses alternatives; Evals assesses saved content.  
  
ALIASES  
/grow-config · /grow-policy · /spec · /prompt · /loom-policy · /loom-control-policy

## /remove

Library: deselect sources. Anthology: unkeep versions. Branches: confirm deletion of versions and descendants. Evaluate: remove selected items, or confirm removal of the open dataset when no item is targeted; sources, policies and Runs remain. Alias: /delete.  
  
ALIASES  
/delete

## /rename

Name a document, conversation or Loom; clear the name to restore its automatic ancestry label.

## /restart

Restart Carla in this workspace; stop active generation and preserve partials.

## /review

Attach a verdict and note to this document or a text range.

## /save

Save the active document or policy edit (editing only).

## /simulator

Configure and inspect raw-document conversation runs.

## /export

Export selected saved items, or the current collection when nothing is selected. Saves content, provenance and judgment metadata to workspace files. Does not judge or train.  
  
ALIASES  
/snapshot

## /visitor

Write a visitor message in a new conversation fork. Open a conversation first.

## /workspace

Open a saved workspace or create a new one.

