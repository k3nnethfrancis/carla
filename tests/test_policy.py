import pytest

from character_lab.policy import validate


def decision(candidates):
    return {
        "reviews": [
            {
                "node": n["id"],
                "decision": "explore",
                "reason": "Develops the image",
                "evidence": n["text"][len(n["prompt"]) :],
            }
            for n in candidates
        ],
        "selected": candidates[-1]["id"],
        "reason": "Most promising direction",
    }


def test_decision_must_cover_real_candidates_and_ground_quotes():
    candidates = [dict(id="a", text="Source new text", prompt="Source")]
    result = decision(candidates)
    assert validate(result, candidates)["selected"] == result["selected"]
    result["reviews"][0]["evidence"] = "invented"
    with pytest.raises(ValueError, match="evidence"):
        validate(result, candidates)
    result = decision(candidates)
    result["selected"] = "other"
    with pytest.raises(ValueError, match="selected"):
        validate(result, candidates)
    result = decision(candidates)
    result["reviews"] = []
    with pytest.raises(ValueError, match="every"):
        validate(result, candidates)


def test_quote_line_wrapping_is_resolved_to_exact_source_span():
    candidates = [dict(id="a", text="SourceA line\nlooks back.", prompt="Source")]
    result = decision(candidates)
    result["reviews"][0]["evidence"] = "A line looks back."
    validated = validate(result, candidates)["reviews"][0]
    assert validated["evidence"] == "A line\nlooks back."
    assert validated["reported_evidence"] == "A line looks back."
    assert (
        candidates[0]["text"][6:][
            validated["evidence_start"] : validated["evidence_end"]
        ]
        == validated["evidence"]
    )
    assert result["reviews"][0]["evidence"] == "A line looks back."
