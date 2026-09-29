# Monitoring probability pilot

Compare System One-compatible classifiers on the same conversation and behavior
specs, outside the Carla app. This starts no conversation generation and changes
no workspace or policy settings.

The initial nine synthetic cases cover **none, blatant, and subtle** examples of
looping, spiraling and harmful language. Every case has expectations for all three
behaviors. These are provisional diagnostic labels for human review, not a
representative benchmark or calibrated target probabilities. Two spiraling cases
explicitly note possible overlap with looping. Prior Carla Jev judgments informed
the scenarios; private trace text and teacher scores are not included.

## Run

From the repository root, with `uv sync --locked` completed:

```sh
# Inspect the workload without model calls or creating results.
uv run python evals/monitoring/run.py --repeats 5 --dry-run

# Both providers, bundled and separate questions: 360 timed requests + warmups.
uv run python evals/monitoring/run.py --repeats 5

# Only the installed local classifier.
uv run python evals/monitoring/run.py --providers local --repeats 5

# An externally managed, loopback System One server and its model alias.
uv run python evals/monitoring/run.py --providers local \
  --local-url http://127.0.0.1:8080 --local-model openjev-latest --repeats 5

# Only hosted Jev, with explicit model choice.
uv run python evals/monitoring/run.py --providers hosted \
  --hosted-model typesafe/jev-1.13 --repeats 5
```

Local automatic management uses Carla's installed, pinned DiffusionGemma/OpenJev
companion. Follow [local judge setup](../../docs/local-judge.md) once beforehand.
The runner starts its own worker on a free port and closes that worker on exit;
it never adopts or terminates an app-owned server. Avoid running another local
judge simultaneously if memory is limited. External URLs must be loopback HTTP.

Hosted calls use Carla's saved OpenRouter credential, or `OPENROUTER_API_KEY` if no
saved credential exists. Running with `hosted` sends these synthetic inputs to
OpenRouter and incurs normal API charges. Credentials are never written to results.
Local-only runs do not use a hosted fallback. These targets must implement
`/v1/systemone` and return `noul` scores; arbitrary chat/GGUF models need an adapter
and cannot be compared simply by naming their GGUF file.

The default is 20 repeats. `--cases path.json` supplies another fixture, `--modes
bundled` or `--modes separate` restricts the comparison, and `--output path` chooses
a new output directory. `--seed` controls request scheduling, not model randomness.
Run `--help` for all options. No request is retried silently.

## What is compared

A **noul** is a separate yes/no judgment returning P(yes). The three behavior
scores need not sum to one. Bundled mode asks all three questions in one request;
separate mode asks each in its own request against identical history. Carla
monitoring now defaults to Separate; this runner continues comparing both modes
by default. The question
wording matches Carla's monitor, including its latest-message and incomplete-text
instructions. Gold labels/rationales are never sent to the models.

Trials are serial, interleaved in seeded random order. Question order and wording
stay fixed, so repeated-call spread does not conflate changing prompts with model
variability. Startup and warmup are recorded separately and excluded from sample
latency. Timed requests use the app's transport and timeout behavior. OpenJev's
sampling/re-read defaults are retained rather than forcing deterministic inference.

Reports include:

- Per-case, per-behavior probability mean, spread and observed range.
- Signed and absolute differences between providers and between request modes.
- Request latency median/p90 and full-case latency (three serial calls for separate
  mode versus one bundled call).
- Secondary thresholded raw agreement and Cohen's kappa, computed over case means
  per behavior at P(yes) > 0.5. Repeats do not increase the number of independent
  cases. Undefined kappa is reported as unavailable.
- Failed requests and missing measurements, rather than treating errors as zero.

Probability agreement is not proof of correctness. A clear gold label does not
supply a uniquely correct probability; subtlety is not a target confidence level.
Five repeats give a preliminary range, not a stable tail-latency estimate. Hosted
network latency is included. Short synthetic cases do not establish long-context
performance, behavior calibration or inference speed alongside llama.cpp.

## Results and provenance

`results/<UTC timestamp>/` is created on demand and gitignored. Each run writes:

- `manifest.json`: frozen cases/specs, request plan, code/environment identity,
  model aliases, runtime pin and setup timings.
- `requests.jsonl`: exact requests, raw responses, probabilities, elapsed times
  and failures, flushed after each call. Warmup is explicitly marked.
- `summary.json` and `summary.md`: machine-readable and readable comparisons.

Model aliases can move; raw response identity is retained, without claiming an
immutable hosted model revision. Raw outputs remain available even if a run fails.
Custom output paths outside `results/` require their own ignore rule. Review your
inputs before using a hosted provider, especially with custom fixtures.
