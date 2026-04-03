"""生成托盘图标（PNG + ICO）到 pkg/tray/assets/ 和 assets/"""

import struct
from io import BytesIO

from PIL import Image, ImageDraw

SIZE = 16
ICO_SIZES = [16, 32, 48]
DIRS = ["pkg/tray/assets", "assets"]


def make_icon(r, g, b, size=SIZE):
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)
    margin = max(1, size // 16)
    draw.ellipse(
        [margin, margin, size - margin - 1, size - margin - 1],
        fill=(r, g, b, 230),
        outline=(255, 255, 255, 200),
        width=margin,
    )
    return img


def png_bytes(img):
    buf = BytesIO()
    img.save(buf, format="PNG")
    return buf.getvalue()


def make_ico(base_img):
    entries = []
    png_datas = []
    for sz in ICO_SIZES:
        resized = base_img.resize((sz, sz), Image.LANCZOS)
        png = png_bytes(resized)
        png_datas.append(png)
        entries.append((sz, png))

    count = len(entries)
    header_size = 6
    entry_size = 16 * count
    data_offset = header_size + entry_size

    ico = struct.pack("<HHH", 0, 1, count)
    for sz, png in entries:
        ico += struct.pack("<BBBB", sz, sz, 0, 0)
        ico += struct.pack("<HH", 1, 32)
        ico += struct.pack("<II", len(png), data_offset)
        data_offset += len(png)

    for png in png_datas:
        ico += png

    return ico


ICONS = {
    "idle": make_icon(158, 158, 158),
    "connected": make_icon(76, 175, 80),
    "error": make_icon(244, 67, 54),
}

DIRS = ["pkg/tray/assets", "assets"]


def main():
    for name, img in ICONS.items():
        png = png_bytes(img)
        ico = make_ico(img)

        for d in DIRS:
            with open(f"{d}/tray-{name}.png", "wb") as f:
                f.write(png)

        with open(f"pkg/tray/assets/tray-{name}.ico", "wb") as f:
            f.write(ico)

        print(f"tray-{name}: png={len(png)}B ico={len(ico)}B")

    print("Done")


if __name__ == "__main__":
    main()
