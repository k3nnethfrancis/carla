"""Descriptive pilot statistics; repeated calls are not independent gold items."""

import math
import statistics
from collections import defaultdict


def distribution(values):
    if not values:
        return dict(n=0, mean=None, sd=None, min=None, max=None, median=None, p90=None)
    ordered = sorted(values)
    index = (len(ordered) - 1) * 0.9
    low = math.floor(index)
    p90 = ordered[low] + (ordered[math.ceil(index)] - ordered[low]) * (index - low)
    return dict(
        n=len(values),
        mean=statistics.mean(values),
        sd=statistics.stdev(values) if len(values) > 1 else 0.0,
        min=min(values),
        max=max(values),
        median=statistics.median(values),
        p90=p90,
    )


def agreement(left, right):
    """Cohen's kappa over paired CASE means thresholded at > .5."""
    if not left:
        return dict(n=0, raw=None, kappa=None)
    n = len(left)
    observed = sum(a == b for a, b in zip(left, right, strict=True)) / n
    p, q = sum(left) / n, sum(right) / n
    expected = p * q + (1 - p) * (1 - q)
    return dict(
        n=n,
        raw=observed,
        kappa=(observed - expected) / (1 - expected) if expected < 1 else None,
    )


def summarize(records, fixture):
    rows = [r for r in records if r["phase"] == "sample"]
    scores, timings, calls = defaultdict(list), defaultdict(list), defaultdict(list)
    expected = {c["id"]: c["expected"] for c in fixture["cases"]}
    for r in rows:
        key = (r["provider"], r["mode"])
        timings[key].append(r["elapsed_seconds"])
        calls[key].append(r)
        for dimension in r["request"]["questions"]:
            scores[(r["provider"], r["mode"], r["case"], dimension)]
        if r["status"] == "complete":
            for dimension, value in r["scores"].items():
                scores[(r["provider"], r["mode"], r["case"], dimension)].append(value)
    probability = []
    for key, values in sorted(scores.items()):
        provider, mode, case, dimension = key
        attempted = sum(
            r["provider"] == provider
            and r["mode"] == mode
            and r["case"] == case
            and dimension in r["request"]["questions"]
            for r in rows
        )
        probability.append(
            dict(
                provider=provider,
                mode=mode,
                case=case,
                dimension=dimension,
                expected=expected[case][dimension],
                attempted=attempted,
                failed=attempted - len(values),
                positive_fraction=sum(v > 0.5 for v in values) / len(values)
                if values
                else None,
                **distribution(values),
            )
        )
    latency = []
    for (provider, mode), values in sorted(timings.items()):
        subset = calls[(provider, mode)]
        grouped = defaultdict(list)
        for r in subset:
            grouped[(r["case"], r["repeat"])].append(r)
        # End-to-end service time for all dimensions, excluding incomplete groups.
        total = [
            sum(r["elapsed_seconds"] for r in group)
            for group in grouped.values()
            if all(r["status"] == "complete" for r in group)
            and set().union(*(set(r["request"]["questions"]) for r in group))
            == set(fixture["questions"])
        ]
        latency.append(
            dict(
                provider=provider,
                mode=mode,
                attempted=len(subset),
                failed=sum(r["status"] != "complete" for r in subset),
                request_seconds=distribution(values),
                successful_case_seconds=distribution(total),
            )
        )
    means = {
        (r["provider"], r["mode"], r["case"], r["dimension"]): r["mean"]
        for r in probability
    }
    comparisons, agreements = [], []
    pairs = [
        ("local", "bundled", "hosted", "bundled"),
        ("local", "separate", "hosted", "separate"),
        ("local", "bundled", "local", "separate"),
        ("hosted", "bundled", "hosted", "separate"),
    ]
    for ap, am, bp, bm in pairs:
        name = f"{ap}/{am} - {bp}/{bm}"
        for dimension in fixture["questions"]:
            left, right = [], []
            for case in fixture["cases"]:
                a, b = (
                    means.get((ap, am, case["id"], dimension)),
                    means.get((bp, bm, case["id"], dimension)),
                )
                if a is None or b is None:
                    continue
                comparisons.append(
                    dict(
                        comparison=name,
                        case=case["id"],
                        dimension=dimension,
                        signed_difference=a - b,
                        absolute_difference=abs(a - b),
                    )
                )
                left.append(a > 0.5)
                right.append(b > 0.5)
            agreements.append(
                dict(comparison=name, dimension=dimension, **agreement(left, right))
            )
    return dict(
        attempted=len(rows),
        failed=sum(r["status"] != "complete" for r in rows),
        probabilities=probability,
        latency=latency,
        probability_differences=comparisons,
        interrater_agreement=agreements,
    )


def markdown(summary):
    lines = [
        "# Monitoring pilot",
        "",
        f"{summary['attempted']} timed requests; {summary['failed']} failed. Failed calls stay in denominators; probability summaries use available successful calls.",
        "",
        "Latency excludes startup/warmup. Case time sums all question requests for a successful case sample; calls run sequentially.",
        "",
        "| Provider / mode | Requests / failed | Request median / p90 (s) | Full case median / p90 (s) |",
        "|---|---:|---:|---:|",
    ]

    def number(x):
        return "—" if x is None else f"{x:.3f}"

    for r in summary["latency"]:
        a, b = r["request_seconds"], r["successful_case_seconds"]
        lines.append(
            f"| {r['provider']} / {r['mode']} | {r['attempted']} / {r['failed']} | {number(a['median'])} / {number(a['p90'])} | {number(b['median'])} / {number(b['p90'])} |"
        )
    lines += [
        "",
        "| Provider / mode | Case | Behavior | Gold | Mean P(yes) | SD | Min–max | Successful / attempted |",
        "|---|---|---|---|---:|---:|---:|---:|",
    ]
    for r in summary["probabilities"]:
        lines.append(
            f"| {r['provider']} / {r['mode']} | {r['case']} | {r['dimension']} | {r['expected']} | {number(r['mean'])} | {number(r['sd'])} | {number(r['min'])}–{number(r['max'])} | {r['n']} / {r['attempted']} |"
        )
    lines += [
        "",
        "## Secondary label agreement",
        "",
        "One pair per case, derived from mean P(yes) > 0.5. Nine cases are nine items, regardless of repeat count. Undefined kappa is shown as —. These results do not establish calibration.",
        "",
        "| Comparison | Behavior | Cases | Agreement | κ |",
        "|---|---|---:|---:|---:|",
    ]
    for r in summary["interrater_agreement"]:
        lines.append(
            f"| {r['comparison']} | {r['dimension']} | {r['n']} | {number(r['raw'])} | {number(r['kappa'])} |"
        )
    lines += [
        "",
        "Per-case signed/absolute probability differences are in summary.json. Positive signed differences mean the first model/mode assigns higher probability.",
        "",
    ]
    return "\n".join(lines)
