# Loom loops and selection

`/loom 2 --turns 2 --loops 2` creates two alternatives. With selection Off,
subsequent loops continue both alternatives in place. With selection On, each
loop assesses the alternatives, chooses a qualifying winner, and forks the next
loop's alternatives from that winner. Every generated path remains saved.
Branches uses the same distinction with output tokens instead of conversation
turns. If the target is a set, each alternative preserves the whole set and
selection chooses between those whole alternatives.

Selection first assesses each enabled behavior using its assessment call mode.
Only candidates passing every enabled behavior reach the branch-selection call.
That separate call chooses one eligible candidate or none. No qualifying
candidate ends the loop sequence. Assessment errors are unknown outcomes,
not evidence that the criteria failed.

## Reading results and retrying

The Simulator tree identifies the loop and marks alternative branches as chosen,
not chosen, criteria not met, or assessment error. Grid headers identify the loop
and branch; generation status remains separate from the policy outcome. A checkmark
still means your manual selection, never the judge's choice.

Use `/inspect` on a Loom conversation, then use Left/Right to open the Selection
tab. Up/Down scroll the saved assessment reasons. Enter opens the raw evidence
list, including exact judge traces and **Retry selection**. Retrying reassesses the last loop's frozen candidates with its original policy
and model configuration, then performs branch choice if possible. It saves a new
attempt, preserves the old attempt, generates no new text, and does not automatically
resume later loops. The latest attempt appears on those same branch outcomes.

Evidence quotes may differ in whitespace only. Carla maps them back to the exact
original span and retains the judge's reported quote. Invented evidence is rejected.

## Behavior spec for new policies

The default describes a property of the candidate rather than asking the behavior
judge to perform selection:

> The document or conversation develops meaningfully from its existing context.
> New material connects to what came before and adds a distinct observation,
> response, or direction. Unusual voices, metaphor, ambiguity, dialogue, lists,
> and nonlinear structure can qualify when meaningful. The text avoids sustained
> empty repetition, unrelated residue, and a collapse into generic boilerplate.
> A conversation's replies respond to the preceding exchange rather than
> repeatedly restarting it. An unfinished passage may qualify. No predetermined
> identity, biography, helpfulness, or conventional style is required.

Existing saved specs and prompt templates are preserved. You can adopt this spec
in a policy's behavior editor. An old run's retry continues using its frozen spec.
