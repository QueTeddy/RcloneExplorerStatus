import math
import os
from struct import pack

ASSET_DIR = os.path.join(os.path.dirname(os.path.dirname(__file__)), 'assets')
os.makedirs(ASSET_DIR, exist_ok=True)

COLORS = {
    'synced': (0, 176, 80),
    'syncing': (0, 120, 215),
    'error': (196, 0, 0),
    'pending': (255, 180, 0),
    'conflict': (230, 126, 34),
    'offline': (120, 120, 120),
    'cloud-only': (92, 92, 92),
}


def set_pixel(pixels: list[list[tuple[int, int, int, int]]], x: int, y: int,
              color: tuple[int, int, int, int]) -> None:
    if 0 <= y < len(pixels) and 0 <= x < len(pixels[y]):
        pixels[y][x] = color


def draw_line(pixels: list[list[tuple[int, int, int, int]]], start: tuple[int, int],
              end: tuple[int, int], color: tuple[int, int, int, int], thickness: int) -> None:
    x0, y0 = start
    x1, y1 = end
    dx, dy = abs(x1 - x0), -abs(y1 - y0)
    step_x = 1 if x0 < x1 else -1
    step_y = 1 if y0 < y1 else -1
    error = dx + dy
    radius = max(0, thickness // 2)
    while True:
        for y_offset in range(-radius, radius + 1):
            for x_offset in range(-radius, radius + 1):
                if x_offset * x_offset + y_offset * y_offset <= radius * radius + 1:
                    set_pixel(pixels, x0 + x_offset, y0 + y_offset, color)
        if x0 == x1 and y0 == y1:
            return
        doubled = 2 * error
        if doubled >= dy:
            error += dy
            x0 += step_x
        if doubled <= dx:
            error += dx
            y0 += step_y


def draw_badge(size: int, name: str, color: tuple[int, int, int]) -> tuple[bytes, bytes]:
    pixels = [[(0, 0, 0, 0) for _ in range(size)] for _ in range(size)]
    center = (size - 1) / 2
    radius = size / 2 - 1
    fill = (*color, 255)
    white = (255, 255, 255, 255)

    for y in range(size):
        for x in range(size):
            distance = math.hypot(x - center, y - center)
            if distance <= radius:
                pixels[y][x] = fill
            if radius - 1.2 <= distance <= radius:
                pixels[y][x] = white

    thickness = max(1, size // 7)
    if name == 'synced':
        draw_line(pixels, (size // 4, size // 2), (size * 2 // 5, size * 2 // 3), white, thickness)
        draw_line(pixels, (size * 2 // 5, size * 2 // 3), (size * 3 // 4, size // 3), white, thickness)
    elif name == 'error':
        draw_line(pixels, (size // 3, size // 3), (size * 2 // 3, size * 2 // 3), white, thickness)
        draw_line(pixels, (size * 2 // 3, size // 3), (size // 3, size * 2 // 3), white, thickness)
    elif name == 'syncing':
        arc_radius = size * 0.23
        points = [(round(center + arc_radius * math.cos(math.radians(angle))),
                   round(center + arc_radius * math.sin(math.radians(angle))))
                  for angle in range(35, 326, 18)]
        for start, end in zip(points, points[1:]):
            draw_line(pixels, start, end, white, max(1, size // 9))
        draw_line(pixels, points[0], (points[0][0] - 2, points[0][1] + 1), white, max(1, size // 9))
        draw_line(pixels, points[0], (points[0][0] - 1, points[0][1] + 2), white, max(1, size // 9))
        draw_line(pixels, points[-1], (points[-1][0] + 2, points[-1][1] - 1), white, max(1, size // 9))
        draw_line(pixels, points[-1], (points[-1][0] + 1, points[-1][1] - 2), white, max(1, size // 9))

    xor_data = bytearray()
    and_data = bytearray()
    mask_stride = ((size + 31) // 32) * 4
    for row in reversed(pixels):
        for red, green, blue, alpha in row:
            xor_data.extend((blue, green, red, alpha))
        mask = bytearray(mask_stride)
        for x, (_, _, _, alpha) in enumerate(row):
            if alpha == 0:
                mask[x // 8] |= 1 << (7 - x % 8)
        and_data.extend(mask)
    return bytes(xor_data), bytes(and_data)


def icon_file_for(name: str, color: tuple[int, int, int]) -> bytes:
    sizes = [16, 32]
    entries = []
    payload = bytearray()
    cursor = 6 + len(sizes) * 16
    for size in sizes:
        xor_data, and_mask = draw_badge(size, name, color)
        image_bytes = xor_data + and_mask
        dib_header = bytearray()
        dib_header.extend(pack('<I', 40))
        dib_header.extend(pack('<i', size))
        dib_header.extend(pack('<i', size * 2))
        dib_header.extend(pack('<H', 1))
        dib_header.extend(pack('<H', 32))
        dib_header.extend(pack('<I', 0))
        dib_header.extend(pack('<I', len(image_bytes)))
        dib_header.extend(pack('<i', 0))
        dib_header.extend(pack('<i', 0))
        dib_header.extend(pack('<I', 0))
        dib_header.extend(pack('<I', 0))
        dib_image = bytes(dib_header) + image_bytes
        entries.append((size, 0, 0, 1, 32, len(dib_image), cursor))
        payload.extend(dib_image)
        cursor += len(dib_image)

    ico = bytearray()
    ico.extend(b'\x00\x00\x01\x00')
    ico.extend(pack('<B', len(sizes)))
    ico.extend(b'\x00')
    for width, _, _, _, _, length, offset in entries:
        ico.extend(pack('<B', width))
        ico.extend(pack('<B', width))
        ico.extend(b'\x00')
        ico.extend(b'\x00')
        ico.extend(pack('<H', 1))
        ico.extend(pack('<H', 32))
        ico.extend(pack('<I', length))
        ico.extend(pack('<I', offset))
    ico.extend(payload)
    return bytes(ico)

for name, color in COLORS.items():
    target = os.path.join(ASSET_DIR, f'{name}.ico')
    with open(target, 'wb') as handle:
        handle.write(icon_file_for(name, color))
    print(f'Wrote {target}')
