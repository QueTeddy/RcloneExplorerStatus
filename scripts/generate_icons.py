"""Generate Windows Explorer overlay icons (OneDrive / Google Drive style).

Explorer composites the full ICO over the file glyph. The badge must be opaque
and readable, while every other pixel is fully transparent (alpha 0). For 32-bpp
icons the AND mask must be all zeros — setting mask bits for transparent pixels
creates the large ghost square seen in Explorer.
"""

from __future__ import annotations

import math
import os
from struct import pack

ASSET_DIR = os.path.join(os.path.dirname(os.path.dirname(__file__)), "assets")
os.makedirs(ASSET_DIR, exist_ok=True)

COLORS = {
    "synced": (0, 170, 70),
    "syncing": (0, 130, 230),
    "error": (210, 35, 35),
    "pending": (255, 185, 0),
    "conflict": (232, 117, 28),
    "offline": (138, 138, 138),
    "cloud-only": (96, 110, 128),
}

# Enough sizes so Explorer picks a crisp glyph for small / medium / large / XL views.
ICON_SIZES = (16, 24, 32, 48, 64, 128, 256)


def blend(
    dst: tuple[int, int, int, int],
    src: tuple[int, int, int, int],
) -> tuple[int, int, int, int]:
    sr, sg, sb, sa = src
    dr, dg, db, da = dst
    if sa <= 0:
        return dst
    if sa >= 255 or da <= 0:
        return src
    out_a = sa + (da * (255 - sa) + 127) // 255
    if out_a <= 0:
        return (0, 0, 0, 0)
    inv = 255 - sa
    out_r = (sr * sa + dr * da * inv // 255 + 127) // out_a
    out_g = (sg * sa + dg * da * inv // 255 + 127) // out_a
    out_b = (sb * sa + db * da * inv // 255 + 127) // out_a
    return (
        max(0, min(255, out_r)),
        max(0, min(255, out_g)),
        max(0, min(255, out_b)),
        max(0, min(255, out_a)),
    )


def put_blend(
    pixels: list[list[tuple[int, int, int, int]]],
    x: int,
    y: int,
    color: tuple[int, int, int, int],
) -> None:
    if 0 <= y < len(pixels) and 0 <= x < len(pixels[y]):
        pixels[y][x] = blend(pixels[y][x], color)


def fill_circle(
    pixels: list[list[tuple[int, int, int, int]]],
    cx: float,
    cy: float,
    radius: float,
    color: tuple[int, int, int],
) -> None:
    r, g, b = color
    min_x = max(0, int(math.floor(cx - radius - 1)))
    max_x = min(len(pixels[0]) - 1, int(math.ceil(cx + radius + 1)))
    min_y = max(0, int(math.floor(cy - radius - 1)))
    max_y = min(len(pixels) - 1, int(math.ceil(cy + radius + 1)))
    for y in range(min_y, max_y + 1):
        for x in range(min_x, max_x + 1):
            distance = math.hypot(x + 0.5 - cx, y + 0.5 - cy)
            coverage = radius + 0.55 - distance
            if coverage <= 0:
                continue
            alpha = 255 if coverage >= 1 else max(0, min(255, int(round(coverage * 255))))
            put_blend(pixels, x, y, (r, g, b, alpha))


def stroke_circle(
    pixels: list[list[tuple[int, int, int, int]]],
    cx: float,
    cy: float,
    radius: float,
    color: tuple[int, int, int],
    width: float,
) -> None:
    r, g, b = color
    half = width / 2
    min_x = max(0, int(math.floor(cx - radius - half - 1)))
    max_x = min(len(pixels[0]) - 1, int(math.ceil(cx + radius + half + 1)))
    min_y = max(0, int(math.floor(cy - radius - half - 1)))
    max_y = min(len(pixels) - 1, int(math.ceil(cy + radius + half + 1)))
    for y in range(min_y, max_y + 1):
        for x in range(min_x, max_x + 1):
            distance = abs(math.hypot(x + 0.5 - cx, y + 0.5 - cy) - radius)
            coverage = half + 0.55 - distance
            if coverage <= 0:
                continue
            alpha = 255 if coverage >= 1 else max(0, min(255, int(round(coverage * 255))))
            put_blend(pixels, x, y, (r, g, b, alpha))


def draw_thick_line(
    pixels: list[list[tuple[int, int, int, int]]],
    start: tuple[float, float],
    end: tuple[float, float],
    color: tuple[int, int, int],
    thickness: float,
) -> None:
    x0, y0 = start
    x1, y1 = end
    length = math.hypot(x1 - x0, y1 - y0)
    if length < 0.001:
        fill_circle(pixels, x0, y0, thickness / 2, color)
        return
    steps = max(int(length * 3), 1)
    radius = thickness / 2
    for i in range(steps + 1):
        t = i / steps
        fill_circle(pixels, x0 + (x1 - x0) * t, y0 + (y1 - y0) * t, radius, color)


def badge_geometry(size: int) -> tuple[float, float, float]:
    """Bottom-right badge ~32% of canvas — readable, still a corner mark."""
    diameter = max(7.0, size * 0.32)
    radius = diameter / 2
    margin = max(0.5, size * 0.03)
    cx = size - margin - radius
    cy = size - margin - radius
    return cx, cy, radius


def draw_symbol(
    pixels: list[list[tuple[int, int, int, int]]],
    name: str,
    cx: float,
    cy: float,
    radius: float,
) -> None:
    white = (255, 255, 255)
    stroke = max(1.2, radius * 0.30)

    if name == "synced":
        p1 = (cx - radius * 0.42, cy + radius * 0.02)
        p2 = (cx - radius * 0.08, cy + radius * 0.38)
        p3 = (cx + radius * 0.46, cy - radius * 0.34)
        draw_thick_line(pixels, p1, p2, white, stroke)
        draw_thick_line(pixels, p2, p3, white, stroke)
    elif name == "error":
        inset = radius * 0.38
        draw_thick_line(pixels, (cx - inset, cy - inset), (cx + inset, cy + inset), white, stroke)
        draw_thick_line(pixels, (cx + inset, cy - inset), (cx - inset, cy + inset), white, stroke)
    elif name == "syncing":
        arc_r = radius * 0.48
        sw = max(1.1, stroke * 0.85)

        def arc(start_deg: float, end_deg: float) -> list[tuple[float, float]]:
            step = 10 if end_deg >= start_deg else -10
            angles = list(range(int(start_deg), int(end_deg) + (1 if step > 0 else -1), step))
            return [
                (
                    cx + arc_r * math.cos(math.radians(angle)),
                    cy + arc_r * math.sin(math.radians(angle)),
                )
                for angle in angles
            ]

        top = arc(-40, 140)
        bottom = arc(140, 320)
        for start, end in zip(top, top[1:]):
            draw_thick_line(pixels, start, end, white, sw)
        for start, end in zip(bottom, bottom[1:]):
            draw_thick_line(pixels, start, end, white, sw)
        if top:
            tip = top[-1]
            draw_thick_line(pixels, tip, (tip[0] - radius * 0.18, tip[1] - radius * 0.02), white, sw)
            draw_thick_line(pixels, tip, (tip[0] - radius * 0.02, tip[1] - radius * 0.18), white, sw)
        if bottom:
            tip = bottom[-1]
            draw_thick_line(pixels, tip, (tip[0] + radius * 0.18, tip[1] + radius * 0.02), white, sw)
            draw_thick_line(pixels, tip, (tip[0] + radius * 0.02, tip[1] + radius * 0.18), white, sw)
    elif name == "pending":
        draw_thick_line(pixels, (cx, cy - radius * 0.35), (cx, cy), white, stroke * 0.85)
        draw_thick_line(pixels, (cx, cy), (cx + radius * 0.28, cy + radius * 0.08), white, stroke * 0.85)
    elif name == "conflict":
        draw_thick_line(pixels, (cx, cy - radius * 0.38), (cx, cy + radius * 0.08), white, stroke)
        fill_circle(pixels, cx, cy + radius * 0.34, max(0.9, radius * 0.14), white)
    elif name == "offline":
        stroke_circle(pixels, cx, cy, radius * 0.42, white, max(1.0, stroke * 0.7))
        draw_thick_line(
            pixels,
            (cx - radius * 0.42, cy + radius * 0.42),
            (cx + radius * 0.42, cy - radius * 0.42),
            white,
            stroke,
        )
    elif name == "cloud-only":
        fill_circle(pixels, cx - radius * 0.18, cy + radius * 0.05, radius * 0.28, white)
        fill_circle(pixels, cx + radius * 0.18, cy + radius * 0.08, radius * 0.24, white)
        fill_circle(pixels, cx, cy - radius * 0.12, radius * 0.30, white)


def draw_badge(size: int, name: str, color: tuple[int, int, int]) -> tuple[bytes, bytes]:
    pixels = [[(0, 0, 0, 0) for _ in range(size)] for _ in range(size)]
    cx, cy, radius = badge_geometry(size)

    # Opaque white rim + filled badge (fully transparent elsewhere).
    fill_circle(pixels, cx, cy, radius + max(0.8, size * 0.03), (255, 255, 255))
    fill_circle(pixels, cx, cy, radius, color)
    draw_symbol(pixels, name, cx, cy, radius)

    xor_data = bytearray()
    # 32-bpp icons: AND mask must be all zeros. Non-zero mask bits cause the
    # large translucent square footprint in Explorer.
    and_data = bytes((((size + 31) // 32) * 4) * size)
    for row in reversed(pixels):
        for red, green, blue, alpha in row:
            alpha = max(0, min(255, alpha))
            if alpha == 0:
                xor_data.extend((0, 0, 0, 0))
            else:
                xor_data.extend((
                    max(0, min(255, blue)),
                    max(0, min(255, green)),
                    max(0, min(255, red)),
                    alpha,
                ))
    return bytes(xor_data), and_data


def icon_file_for(name: str, color: tuple[int, int, int]) -> bytes:
    entries: list[tuple[int, int, int]] = []
    payload = bytearray()
    cursor = 6 + len(ICON_SIZES) * 16
    for size in ICON_SIZES:
        xor_data, and_mask = draw_badge(size, name, color)
        image_bytes = xor_data + and_mask
        dib_header = bytearray()
        dib_header.extend(pack("<I", 40))
        dib_header.extend(pack("<i", size))
        dib_header.extend(pack("<i", size * 2))
        dib_header.extend(pack("<H", 1))
        dib_header.extend(pack("<H", 32))
        dib_header.extend(pack("<I", 0))
        dib_header.extend(pack("<I", len(image_bytes)))
        dib_header.extend(pack("<i", 0))
        dib_header.extend(pack("<i", 0))
        dib_header.extend(pack("<I", 0))
        dib_header.extend(pack("<I", 0))
        dib_image = bytes(dib_header) + image_bytes
        entries.append((size, len(dib_image), cursor))
        payload.extend(dib_image)
        cursor += len(dib_image)

    ico = bytearray()
    ico.extend(b"\x00\x00\x01\x00")
    ico.extend(pack("<H", len(ICON_SIZES)))
    for width, length, offset in entries:
        ico.extend(pack("<B", 0 if width >= 256 else width))
        ico.extend(pack("<B", 0 if width >= 256 else width))
        ico.extend(b"\x00")
        ico.extend(b"\x00")
        ico.extend(pack("<H", 1))
        ico.extend(pack("<H", 32))
        ico.extend(pack("<I", length))
        ico.extend(pack("<I", offset))
    ico.extend(payload)
    return bytes(ico)


for name, color in COLORS.items():
    target = os.path.join(ASSET_DIR, f"{name}.ico")
    with open(target, "wb") as handle:
        handle.write(icon_file_for(name, color))
    print(f"Wrote {target}")
