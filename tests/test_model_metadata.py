import struct

from character_lab.model_metadata import native_context


def test_native_context_from_gguf_header(tmp_path):
    def string(value):
        data = value.encode()
        return struct.pack("<Q", len(data)) + data

    path = tmp_path / "model.gguf"
    path.write_bytes(
        b"GGUF"
        + struct.pack("<IQQ", 3, 0, 3)
        + string("general.name")
        + struct.pack("<I", 8)
        + string("Test model")
        + string("general.architecture")
        + struct.pack("<I", 8)
        + string("test")
        + string("test.context_length")
        + struct.pack("<II", 4, 131072)
    )
    assert native_context({"path": str(path)}) == 131072
    path.write_bytes(b"invalid")
    assert native_context({"path": str(path)}) is None
    assert native_context({"path": str(tmp_path / "missing")}) is None
