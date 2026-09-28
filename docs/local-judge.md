# Local DiffusionGemma judge

Carla can monitor generations and evaluate saved documents/conversations with
DiffusionGemma on Apple Silicon. It uses [OpenJev](https://github.com/razorback16/openjev),
an independent implementation of Jev's System One classification API, with MLX.
These are **DiffusionGemma weights, not TypeSafe's Jev weights**. Matching the API
does not establish equivalent judgments or calibrated probabilities.

Base-model generation and instruct-model selection still use llama.cpp. This
optional companion service has its own dependencies and model residency.

## Start locally

From the Carla repository, in a second terminal:

```sh
./scripts/local-judge.sh
```

Requires Apple Silicon, `uv`, and internet access for the initial installation
and model download. The script pins OpenJev and the
[MLX 4-bit checkpoint](https://huggingface.co/mlx-community/diffusiongemma-26B-A4B-it-4bit)
to specific commits. Weights use the Hugging Face cache, outside the repository;
Carla's GGUF registry is unchanged. Allow about **17 GB of disk space** plus runtime
installation space, and at least **16 GB free RAM for the classifier alone**.
The generator, its KV cache, and other applications need additional memory.

Wait for `Application startup complete`. The service binds `127.0.0.1:8080`.
To use another port: `OPENJEV_PORT=8081 ./scripts/local-judge.sh`. Stop it with
Ctrl+C when finished; Carla does not manage or unload this separate process.
After installation and download, run fully from the cache:

```sh
UV_OFFLINE=1 HF_HUB_OFFLINE=1 ./scripts/local-judge.sh
```

Both the model and `uv` dependency environment must already be cached.

The launcher disables OpenJev model routing and binds to loopback, with no API
key. It sets Hugging Face/Transformers offline mode before loading the model.
Inference stays local; the initial dependency/model acquisition uses the internet.
If you manage the service yourself, keep it on loopback and disable remote model
routes. Carla cannot attest to what a separately managed service does internally.

## Use in Carla

**Live monitoring:** `/policy` → Monitoring → **DiffusionGemma (local)**.
The Server and Model rows configure the local service; defaults are
`http://127.0.0.1:8080` and `openjev-latest`. No API key is requested.
Heartbeat, behavior specs, detection thresholds and Warn/Stop actions work exactly
as for Jev. Monitoring remains Off in a new workspace until explicitly enabled.
The same policy covers document continuations and Character replies; Visitor
replies are not monitored separately.

**Whole-item evaluations:** `/policy` → Judge configurations → New judge →
**DiffusionGemma (local)**. Add criteria, choose the pass threshold, and check the
Server/Model settings. Add that judge to a named evaluation in Evaluate, then run
`/eval` on selected material or `/loom … --eval "evaluation name"`. Each result
freezes its judge configuration, including the endpoint, alongside the exact
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
