"""Read the native context from GGUF metadata without loading model tensors."""

import struct
from functools import lru_cache
from pathlib import Path


@lru_cache(maxsize=32)
def _context(path, size, modified):
    formats = {
        0: "B",
        1: "b",
        2: "H",
        3: "h",
        4: "I",
        5: "i",
        6: "f",
        7: "?",
        10: "Q",
        11: "q",
        12: "d",
    }
    with open(path, "rb") as stream:

        def number(fmt):
            return struct.unpack("<" + fmt, stream.read(struct.calcsize(fmt)))[0]

        def string():
            return stream.read(number("Q")).decode("utf-8")

        def skip(kind):
            if kind == 8:
                stream.seek(number("Q"), 1)
            elif kind == 9:
                element, count = number("I"), number("Q")
                if element in formats:
                    stream.seek(struct.calcsize(formats[element]) * count, 1)
                else:
                    for _ in range(count):
                        skip(element)
            else:
                stream.seek(struct.calcsize(formats[kind]), 1)

        if stream.read(4) != b"GGUF" or number("I") not in (2, 3):
            return None
        number("Q")  # Tensor count; only metadata is needed.
        entries = number("Q")
        architecture = None
        contexts = {}
        for _ in range(entries):
            key, kind = string(), number("I")
            if key == "general.architecture" and kind == 8:
                architecture = string()
            elif key.endswith(".context_length") and kind in formats:
                contexts[key] = number(formats[kind])
            else:
                skip(kind)
            if architecture and architecture + ".context_length" in contexts:
                value = contexts[architecture + ".context_length"]
                return value if type(value) is int and value > 0 else None
    return None


def native_context(model):
    path = Path(model["path"])
    try:
        stat = path.stat()
        return _context(str(path), stat.st_size, stat.st_mtime_ns)
    except (OSError, ValueError, KeyError, struct.error):
        return None
