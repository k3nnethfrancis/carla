"""Release identity must remain immutable across retries and branch promotions."""

import importlib.util
from pathlib import Path

import pytest

spec = importlib.util.spec_from_file_location(
    "release", Path(__file__).parents[1] / "scripts/release.py"
)
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


def test_first_release_and_next_patch():
    assert release.release_tag("0.1.0", "new", {}) == "v0.1.0"
    assert release.release_tag("0.1.1", "new", {"v0.1.0": "old"}) == "v0.1.1"


def test_retry_preserves_tag_identity():
    assert release.release_tag("0.1.0", "same", {"v0.1.0": "same"}) == "v0.1.0"
    with pytest.raises(ValueError, match="another commit"):
        release.release_tag("0.1.0", "new", {"v0.1.0": "old"})


@pytest.mark.parametrize("version", ["0.0.9", "0.1.1"])
def test_release_cannot_go_backwards(version):
    with pytest.raises(ValueError, match="newer"):
        release.release_tag(version, "new", {"v0.2.0": "old"})


@pytest.mark.parametrize("version", ["latest", "v0.1.0", "0.1", "0.1.0-dev"])
def test_release_requires_plain_numeric_version(version):
    with pytest.raises(ValueError, match="numeric"):
        release.release_tag(version, "new", {})
