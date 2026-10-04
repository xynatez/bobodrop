"""Verify source bytes, ICO frames, and linked Windows icon resources without launching."""
import hashlib
import struct
import sys
from pathlib import Path

from PIL import Image

folder = Path(__file__).resolve().parent
project = folder.parent
source = folder / "bobodrop-source.png"
sizes = {16, 24, 32, 48, 64, 128, 256}
digest = lambda data: hashlib.sha256(data).hexdigest()
original = source.read_bytes()
with Image.open(source) as supplied:
    assert supplied.convert("RGBA").getchannel("A").getextrema()[0] == 0, "Expected supplied transparent logo"
for path in (folder / "bobodrop.png", project / "web/logo.png"):
    assert path.read_bytes() == original, path
print("PNG SHA256:", digest(original))

ico = (folder / "bobodrop.ico").read_bytes()
assert ico == (project / "web/favicon.ico").read_bytes()
assert struct.unpack_from("<HHH", ico) == (0, 1, 7)
frames = []
for index in range(7):
    width, height, _, _, _, _, length, offset = struct.unpack_from("<BBBBHHII", ico, 6 + index * 16)
    assert (width or 256) == (height or 256)
    frames.append(ico[offset:offset + length])
with Image.open(folder / "bobodrop.ico") as image:
    assert image.ico.sizes() == {(size, size) for size in sizes}
    for size in sizes:
        frame = image.ico.getimage((size, size))
        frame.load()
        assert frame.convert("RGBA").getchannel("A").getextrema()[0] == 0, "Icon background lost transparency"
print("ICO: seven decoded frames, 16/24/32/48/64/128/256; SHA256:", digest(ico))


def verify_resource(path):
    data = path.read_bytes()
    coff = struct.unpack_from("<I", data, 60)[0] + 4 if data[:2] == b"MZ" else 0
    machine, sections, _, _, _, optional_size, _ = struct.unpack_from("<HHIIIHH", data, coff)
    assert machine == 0x8664, "Expected AMD64 COFF"
    section_table = coff + 20 + optional_size
    for index in range(sections):
        header = section_table + index * 40
        if data[header:header + 8].rstrip(b"\0") == b".rsrc":
            _, virtual_address, _, raw_offset = struct.unpack_from("<IIII", data, header + 8)
            break
    else:
        raise AssertionError("Missing .rsrc section")

    resources = {}

    def walk(offset, keys=()):
        named, ids = struct.unpack_from("<HH", data, raw_offset + offset + 12)
        for index in range(named + ids):
            key, child = struct.unpack_from("<II", data, raw_offset + offset + 16 + index * 8)
            assert not key & 0x80000000, "Expected numeric resource ID"
            if child & 0x80000000:
                walk(child & 0x7fffffff, keys + (key,))
            else:
                address, length = struct.unpack_from("<II", data, raw_offset + child)
                start = raw_offset + address - virtual_address
                resources[keys + (key,)] = data[start:start + length]

    walk(0)
    groups = [payload for keys, payload in resources.items() if keys[:2] == (14, 1)]
    assert len(groups) == 1, "Expected RT_GROUP_ICON ID 1 for LoadIconW"
    group = groups[0]
    assert struct.unpack_from("<HHH", group) == (0, 1, 7)
    for index, expected in enumerate(frames):
        length, icon_id = struct.unpack_from("<IH", group, 6 + index * 14 + 8)
        icons = [payload for keys, payload in resources.items() if keys[:2] == (3, icon_id)]
        assert icons == [expected] and length == len(expected), "Embedded icon differs from ICO"
    print(path.name + ": AMD64, RT_GROUP_ICON ID 1, all seven exact ICO payloads; SHA256:", digest(data))


verify_resource(project / "resource_windows.syso")
for argument in sys.argv[1:]:
    verify_resource(Path(argument))
