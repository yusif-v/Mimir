#!/usr/bin/env python3
"""
Mimir logo Braille art generator - FINAL v9.
Three-part layout with pixel-perfect rune extraction and equal spacing.

Layout:
  Part 1: Yggdrasil tree (above upper h-line)
  Part 2: Runes M I M I R (between h-lines, equal spacing, M2 centered)
  Part 3: Diamond knot (below lower h-line)

Each part is independently extracted from the source image.
Runes are repositioned for equal inter-rune spacing.
"""

import os
import sys

from PIL import Image

# Braille dot layout: (bit_mask, dx, dy) within a 2x4 cell
DOTS = [
    (0x01, 0, 0), (0x02, 0, 1), (0x04, 0, 2), (0x40, 0, 3),
    (0x08, 1, 0), (0x10, 1, 1), (0x20, 1, 2), (0x80, 1, 3),
]


def render_region(img, rx, ry, rw, rh, out_w, threshold=80):
    """Render an image region to Braille text with specified output width in cells."""
    w, h = img.size
    pixels = list(img.getdata())

    def px(x, y):
        if 0 <= x < w and 0 <= y < h:
            return pixels[y * w + x]
        return 0

    pp_x = rw / out_w
    pp_y = pp_x * 2
    out_h = max(1, round(rh / pp_y))

    rows = []
    for cy in range(out_h):
        line = []
        for cx in range(out_w):
            sx = round(rx + cx * pp_x)
            sy = round(ry + cy * pp_y)
            d = 0
            for bit, ddx, ddy in DOTS:
                if px(sx + ddx, sy + ddy) > threshold:
                    d |= bit
            line.append(chr(0x2800 + d))
        rows.append(''.join(line))
    return rows


def center_pad(rows, target_w):
    """Center-pad rows to target width with Braille blanks."""
    result = []
    for row in rows:
        if len(row) < target_w:
            pad = (target_w - len(row)) // 2
            row = '\u2800' * pad + row + '\u2800' * (target_w - len(row) - pad)
        result.append(row[:target_w])
    return result


def main():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    img_path = os.path.expanduser('~/.hermes/images/clip_20260618_221943_1.png')

    if not os.path.exists(img_path):
        print(f"Error: Image not found: {img_path}", file=sys.stderr)
        sys.exit(1)

    THRESHOLD = 80
    OUTPUT_W = 100
    img = Image.open(img_path).convert('L')
    w, h = img.size
    cx = w / 2  # 627

    # ========================================
    # Measured rune positions from source image
    # ========================================
    # Rune body y range: 612-771
    rune_y1, rune_y2 = 612, 771
    rune_h = rune_y2 - rune_y1 + 1  # 160px

    # Exact rune x ranges (from pixel analysis):
    # M1: 263-365 (w=103, center=314)
    # I1: 469-485 (w=17, center=477)
    # M2: 586-693 (w=108, center=640)
    # I2: 800-816 (w=17, center=808)
    # R:  923-1002 (w=80, center=962)
    #
    # Gaps between rune edges: 103, 100, 106, 106 (avg ~104)
    # Total span: 263 to 1002 = 740px

    rune_defs = [
        # (name, x1, x2)
        ('M1', 263, 365),
        ('I1', 469, 485),
        ('M2', 586, 693),
        ('I2', 800, 816),
        ('R',  923, 1002),
    ]

    # ========================================
    # PART 1: TREE
    # ========================================
    tree_x1, tree_y1 = 491, 246
    tree_w, tree_h = 276, 322
    tree_rows = render_region(img, tree_x1, tree_y1, tree_w, tree_h, OUTPUT_W // 2, THRESHOLD)
    tree_rows = center_pad(tree_rows, OUTPUT_W)

    # ========================================
    # PART 2: RUNES with equal spacing
    # ========================================
    # Render each rune individually at high resolution
    # Use ~6 pixels per Braille cell for runes (good detail)
    rune_renders = []
    rune_cell_widths = []

    for name, rx1, rx2 in rune_defs:
        rw = rx2 - rx1 + 1
        # Cell width: each cell covers ~6 source pixels
        cell_w = max(3, round(rw / 6))
        rows = render_region(img, rx1, rune_y1, rw, rune_h, cell_w, THRESHOLD)
        rune_renders.append(rows)
        rune_cell_widths.append(cell_w)

    # Calculate equal spacing
    # Total rune cells + gaps must = OUTPUT_W
    total_rune_cells = sum(rune_cell_widths)
    num_inner_gaps = len(rune_defs) - 1  # 4 gaps between 5 runes

    # Available space for gaps
    available_for_gaps = OUTPUT_W - total_rune_cells
    # Each inner gap gets equal share
    inner_gap = available_for_gaps // (num_inner_gaps + 2)  # +2 for left/right padding
    # Distribute remainder to left/right padding
    remainder = available_for_gaps - inner_gap * num_inner_gaps
    left_pad = remainder // 2
    right_pad = remainder - left_pad

    # Build gap array: [left_pad, gap, gap, gap, gap, right_pad]
    gaps = [left_pad]
    for i in range(num_inner_gaps):
        gaps.append(inner_gap)
    gaps.append(right_pad)

    # Verify total
    total = sum(rune_cell_widths) + sum(gaps)
    # Adjust if off by 1-2 cells
    if total < OUTPUT_W:
        gaps[-1] += OUTPUT_W - total
    elif total > OUTPUT_W:
        gaps[-1] -= total - OUTPUT_W

    print(f"Rune cell widths: {rune_cell_widths} (total={total_rune_cells})", file=sys.stderr)
    print(f"Gaps: {gaps} (total={sum(gaps)})", file=sys.stderr)
    print(f"Grand total: {total_rune_cells + sum(gaps)}", file=sys.stderr)

    # Verify M2 is centered
    m2_left = gaps[0] + rune_cell_widths[0] + gaps[1] + rune_cell_widths[1] + gaps[2]
    m2_center = m2_left + rune_cell_widths[2] // 2
    print(f"M2 center cell: {m2_center} (target: {OUTPUT_W // 2})", file=sys.stderr)

    # Assemble rune rows
    max_rune_h = max(len(r) for r in rune_renders)
    rune_rows = []
    for row_idx in range(max_rune_h):
        parts = []
        for i in range(len(rune_renders)):
            parts.append('\u2800' * gaps[i])
            if row_idx < len(rune_renders[i]):
                parts.append(rune_renders[i][row_idx])
            else:
                parts.append('\u2800' * rune_cell_widths[i])
        parts.append('\u2800' * gaps[-1])
        line = ''.join(parts)
        # Ensure exact width
        if len(line) > OUTPUT_W:
            line = line[:OUTPUT_W]
        elif len(line) < OUTPUT_W:
            line += '\u2800' * (OUTPUT_W - len(line))
        rune_rows.append(line)

    # ========================================
    # PART 3: DIAMOND KNOT
    # ========================================
    dx1, dy1 = 581, 814
    dw, dh = 95, 185
    diamond_rows = render_region(img, dx1, dy1, dw, dh, OUTPUT_W // 3, THRESHOLD)
    diamond_rows = center_pad(diamond_rows, OUTPUT_W)

    # ========================================
    # ASSEMBLE
    # ========================================
    sep = '─' * OUTPUT_W

    # Calculate label x-positions (center of each rune in the output)
    label_positions = []
    x_pos = gaps[0]
    for i, cw in enumerate(rune_cell_widths):
        label_positions.append(x_pos + cw // 2)
        x_pos += cw + gaps[i + 1]

    rune_labels_u = ['ᛗ', 'ᛁ', 'ᛗ', 'ᛁ', 'ᚱ']
    rune_labels_a = ['M', 'I', 'M', 'I', 'R']

    label_line_u = list(' ' * OUTPUT_W)
    label_line_a = list(' ' * OUTPUT_W)
    for lx, lu, la in zip(label_positions, rune_labels_u, rune_labels_a):
        if 0 <= lx < OUTPUT_W:
            label_line_u[lx] = lu
            label_line_a[lx] = la

    all_rows = []
    all_rows.extend(tree_rows)
    all_rows.append('')
    all_rows.extend(rune_rows)
    all_rows.append('')
    all_rows.extend(diamond_rows)
    all_rows.append('')
    all_rows.append(sep)
    all_rows.append(''.join(label_line_u))
    all_rows.append(''.join(label_line_a))
    all_rows.append(sep)

    output = '\n'.join(all_rows)
    print(output)

    out_path = os.path.join(script_dir, 'logo-braille.txt')
    with open(out_path, 'w', encoding='utf-8') as f:
        f.write(output)
    print(f'\nSaved to: {out_path}', file=sys.stderr)


if __name__ == '__main__':
    main()
