#!/usr/bin/env python3
"""
High-quality image-to-Braille converter for the Mimir logo.
Reads the original PNG and converts to Braille art.

Braille Unicode: U+2800 base, 8 dots per cell in 2x4 grid:
  d1 d4  row 1  (bits 0x01, 0x08)
  d2 d5  row 2  (bits 0x02, 0x10)
  d3 d6  row 3  (bits 0x04, 0x20)
  d7 d8  row 4  (bits 0x40, 0x80)
"""

import os
import sys

from PIL import Image


def image_to_braille(img_path, output_width=100, threshold=80):
    """Convert an image to Braille art."""
    img = Image.open(img_path).convert('L')
    
    img_w, img_h = img.size
    pixel_w = output_width * 2
    pixel_h = int(round(img_h * (pixel_w / img_w)))
    cell_h = pixel_h // 4
    
    img_resized = img.resize((pixel_w, pixel_h), Image.LANCZOS)
    pixels = img_resized.load()
    
    rows = []
    for cr in range(cell_h):
        line_chars = []
        for cc in range(output_width):
            x = cc * 2
            y = cr * 4
            
            d1 = 1 if pixels[x, y] > threshold else 0
            d2 = 1 if y+1 < pixel_h and pixels[x, y+1] > threshold else 0
            d3 = 1 if y+2 < pixel_h and pixels[x, y+2] > threshold else 0
            d4 = 1 if x+1 < pixel_w and pixels[x+1, y] > threshold else 0
            d5 = 1 if x+1 < pixel_w and y+1 < pixel_h and pixels[x+1, y+1] > threshold else 0
            d6 = 1 if x+1 < pixel_w and y+2 < pixel_h and pixels[x+1, y+2] > threshold else 0
            d7 = 1 if y+3 < pixel_h and pixels[x, y+3] > threshold else 0
            d8 = 1 if x+1 < pixel_w and y+3 < pixel_h and pixels[x+1, y+3] > threshold else 0
            
            v = d1*1 + d2*2 + d3*4 + d4*8 + d5*16 + d6*32 + d7*64 + d8*128
            line_chars.append(chr(0x2800 + v))
        rows.append(''.join(line_chars))
    
    return '\n'.join(rows)


def main():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    img_path = os.path.expanduser('~/.hermes/images/clip_20260618_221943_1.png')
    
    if not os.path.exists(img_path):
        print(f"Error: Image not found: {img_path}", file=sys.stderr)
        sys.exit(1)
    
    # Tuned for the Mimir logo (high contrast, logo elements at luminance 100-200)
    art = image_to_braille(img_path, output_width=100, threshold=80)
    
    # Trim empty rows from top and bottom
    lines = art.split('\n')
    while lines and all(c == '\u2800' for c in lines[0]):
        lines.pop(0)
    while lines and all(c == '\u2800' for c in lines[-1]):
        lines.pop()
    
    art = '\n'.join(lines)
    width = 100
    
    # Labels
    sep = '─' * width
    label_line = ' ' * 36 + 'ᛗ' + ' ' * 7 + 'ᛁ' + ' ' * 7 + 'ᛗ' + ' ' * 7 + 'ᛁ' + ' ' * 7 + 'ᚱ'
    name_line = ' ' * 35 + 'M' + ' ' * 9 + 'I' + ' ' * 9 + 'M' + ' ' * 9 + 'I' + ' ' * 9 + 'R'
    
    output = '\n'.join(['', art, '', sep, label_line, name_line, sep, ''])
    
    print(output)
    
    out_path = os.path.join(script_dir, 'logo-braille.txt')
    with open(out_path, 'w', encoding='utf-8') as f:
        f.write(output)
    print(f'Saved to: {out_path}', file=sys.stderr)


if __name__ == '__main__':
    main()
