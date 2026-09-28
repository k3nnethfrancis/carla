"""Provider credentials stay private and are isolated from research records."""

import json
import stat

import pytest

from character_lab import credentials


def test_private_saved_key_and_environment_fallback(monkeypatch, local_data):
    monkeypatch.delenv("OPENROUTER_API_KEY", raising=False)
    assert credentials.openrouter_key() == ("", "")
    monkeypatch.setenv("OPENROUTER_API_KEY", "env-test-only")
    assert credentials.openrouter_key() == ("env-test-only", "environment")
    credentials.save_openrouter_key("saved-test-only")
    assert credentials.openrouter_key() == ("saved-test-only", "saved")
    path = local_data / "credentials.json"
    assert stat.S_IMODE(path.stat().st_mode) == 0o600
    credentials.save_openrouter_key("replacement-test-only")
    assert json.loads(path.read_text())["openrouter_api_key"] == "replacement-test-only"
    for invalid in (None, "", " ", "key with spaces"):
        with pytest.raises(ValueError):
            credentials.save_openrouter_key(invalid)
    assert credentials.openrouter_key()[0] == "replacement-test-only"
