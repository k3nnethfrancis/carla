# Local DiffusionGemma judge

Carla can monitor generations and evaluate saved documents/conversations with
DiffusionGemma on Apple Silicon. It uses [OpenJev](https://github.com/razorback16/openjev),
an independent implementation of Jev's System One classification API, with MLX.
These are **DiffusionGemma weights, not TypeSafe's Jev weights**. Matching the API
does not establish equivalent judgments or calibrated probabilities.

Base-model generation and instruct-model selection still use llama.cpp. This
optional companion service has its own dependencies and model residency.

## Install once

On Apple Silicon, install the optional runtime and checkpoint from the Carla
repository:

```sh
./scripts/local-judge.sh
```

Requires `uv` and internet access for the initial installation and model download.
Wait for `Application startup complete`, then stop that setup process with Ctrl+C.
Carla subsequently starts the cached judge automatically when a local monitoring
or evaluation request needs it. No server address or port is required.

The installer pins OpenJev and the
[MLX 4-bit checkpoint](https://huggingface.co/mlx-community/diffusiongemma-26B-A4B-it-4bit)
to specific commits. Weights use the Hugging Face cache outside the repository;
Carla's GGUF registry is unchanged. Allow about **17 GB of disk space** plus runtime
installation space, and at least **16 GB free RAM for the classifier alone**.
The generator, its KV cache, and other applications need additional memory.

Automatic startup is cache-only: both the model and `uv` dependency environment
must already be installed. Missing dependencies produce an installation message,
not a background download. The worker disables model routing and credential use;
its dependencies and model load with offline mode enabled.

Carla asks the operating system for an available loopback port and retains that
socket through startup. Concurrent checks share one owned worker per Carla
backend. Quitting or restarting Carla stops that worker; a watchdog also handles
an abruptly terminated backend. Carla never discovers, adopts or kills unrelated
servers. Multiple separate Carla instances have their own workers and memory use.
Startup can take longer than a subsequent classification, with a 180-second bound.

Saved explicit loopback addresses remain supported for separately managed servers.
Selecting DiffusionGemma again switches to automatic management. Such external
servers are not started or stopped by Carla. The actual endpoint used is recorded
in each judgment, even when the saved setting is `auto`.

## Use in Carla

**Live monitoring:** `/policy` → Monitoring → a policy. Enable monitoring and choose
**DiffusionGemma (classifier)**. To change it later, open Judge → Model.
The local worker starts automatically using `openjev-latest`. No API key is requested.
Heartbeat, behavior specs, detection thresholds and Warn/Stop actions work exactly
as for Jev. Call mode defaults to Separate (one request per enabled behavior).
Bundled is available after a warning confirmation; fewer requests can be faster,
but shared question context can change judgments. Monitoring remains Off in a new
workspace until explicitly enabled.
The same policy covers document continuations and Character replies; Visitor
replies are not monitored separately.

**Whole-item evaluations:** open Evaluate → Policies → a policy. Add a
**DiffusionGemma (classifier)** judge and define specs and pass thresholds in the
sibling **Behaviors** panel. With multiple judges, all assess the same enabled specs. Choose its Call mode at the judge level, then run `/eval "policy name"` on selected data or
`/loom … --eval "policy name"`. Each result
freezes its judge configuration, including automatic or explicit endpoint mode, alongside the exact
request, response, provider and elapsed time. Later policy changes do not rewrite it.

Selection during multi-loop Loom still uses the configured local instruct model;
this addition does not replace its candidate/evidence contract.

## Limits and failures

OpenJev reads probability mass from DiffusionGemma's output logits for named
questions. Carla uses its yes/no (`noul`) scores, not a generated numerical answer.
OpenJev's default sampling/re-read behavior is retained; Carla does not silently
replace it with a cheaper or truncated judgment. Validate your behavior specs
against examples before trusting automated Stop actions or training selection.

MLX reads run serially in OpenJev, even if several Carla conversations run at once.
Partial monitoring does not block token streaming, and Carla coalesces pending
checks per conversation; the next turn waits for its final check. The classifier
shares GPU/memory bandwidth with llama.cpp, so enabling it can reduce generation
throughput. Begin with after-reply checks and measure before lowering the interval.

Carla gives local requests a 120-second timeout (3 seconds to connect), with no
retries or remote fallback. A missing service, timeout, oversized input or invalid
score remains an explicit unavailable/error result. Monitoring errors do not stop
generation; evaluation errors cannot create a pass or mark a new item for training.
Only a configured Stop rule applied to a successful classification stops output.
Full history/text is sent: the service's context limit produces an error instead
of silently truncating evidence (OpenJev defaults to 32,768 prompt tokens).

This launcher covers Apple Silicon. On other hardware, OpenJev documents a vLLM
backend; run it separately with the same loopback API. Carla's transport is shared,
but that deployment is not covered by the Mac launcher or its local QA.
