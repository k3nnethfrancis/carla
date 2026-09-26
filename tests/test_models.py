import json

import pytest

from character_lab.models import available_models, load_models
from character_lab.runtime import DEFAULT_MODEL


def config():
    return dict(alias="local-base", kind="base", path="weights.gguf", port=18986)


def test_config_relative_paths_and_workspace_overrides(local_data):
    path = local_data / "models.json"
    path.write_text(json.dumps([config()]))
    model = load_models(path)[0]
    assert model["path"] == str(local_data / "weights.gguf")
    assert model["url"] == "http://127.0.0.1:18986"
    assert available_models([{**model, "context": 4096}])[0]["context"] == 4096


@pytest.mark.parametrize(
    "value",
    [
        [],
        [config(), config()],
        [{**config(), "port": 0}],
        [{**config(), "url": "http://127.0.0.1:18986@remote.invalid"}],
        [{**config(), "kind": "instruct"}],
        [{**config(), "context": -1}],
    ],
)
def test_invalid_configuration_fails(value, local_data):
    path = local_data / "models.json"
    path.write_text(json.dumps(value))
    with pytest.raises(ValueError):
        load_models(path)


def test_missing_registry_has_an_explicit_unconfigured_model(local_data):
    assert available_models() == [DEFAULT_MODEL]
    saved = {**DEFAULT_MODEL, "alias": "custom"}
    assert available_models([saved]) == [saved]
