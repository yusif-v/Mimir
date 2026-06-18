#!/usr/bin/env python3
"""
Braille art generator for the Mimir logo - FINAL.
Generates the Braille art and saves to logo-braille.txt with rune labels.
"""


class Canvas:
    def __init__(self, pw, ph):
        self.pw = pw
        self.ph = ph
        self.cx = pw // 2
        self.grid = [[0]*pw for _ in range(ph)]
    
    def set(self, x, y):
        if 0 <= x < self.pw and 0 <= y < self.ph:
            self.grid[y][x] = 1
    
    def hline(self, x1, x2, y):
        for x in range(max(0,x1), min(self.pw, x2+1)):
            self.set(x, y)
    
    def vline(self, x, y1, y2):
        for y in range(max(0,y1), min(self.ph, y2+1)):
            self.set(x, y)
    
    def line(self, x1, y1, x2, y2):
        dx, dy = abs(x2-x1), abs(y2-y1)
        sx = 1 if x1 < x2 else -1
        sy = 1 if y1 < y2 else -1
        err = dx - dy
        while True:
            self.set(x1, y1)
            if x1 == x2 and y1 == y2:
                break
            e2 = 2*err
            if e2 > -dy:
                err -= dy
                x1 += sx
            if e2 < dx:
                err += dx
                y1 += sy
    
    def to_braille(self):
        cw = (self.pw + 1) // 2
        ch = (self.ph + 3) // 4
        rows = []
        for cr in range(ch):
            line_chars = []
            for cc in range(cw):
                x, y = cc*2, cr*4
                d1 = self.grid[y][x] if y < self.ph and x < self.pw else 0
                d2 = self.grid[y+1][x] if y+1 < self.ph and x < self.pw else 0
                d3 = self.grid[y+2][x] if y+2 < self.ph and x < self.pw else 0
                d4 = self.grid[y][x+1] if y < self.ph and x+1 < self.pw else 0
                d5 = self.grid[y+1][x+1] if y+1 < self.ph and x+1 < self.pw else 0
                d6 = self.grid[y+2][x+1] if y+2 < self.ph and x+1 < self.pw else 0
                d7 = self.grid[y+3][x] if y+3 < self.ph and x < self.pw else 0
                d8 = self.grid[y+3][x+1] if y+3 < self.ph and x+1 < self.pw else 0
                v = d1*1+d2*2+d3*4+d4*8+d5*16+d6*32+d7*64+d8*128
                line_chars.append(chr(0x2800+v))
            rows.append(''.join(line_chars))
        return '\n'.join(rows)


def main():
    PW, PH = 96, 60
    c = Canvas(PW, PH)
    cx = c.cx  # 48
    
    # YGGDRASIL TREE
    c.vline(cx, 1, 13)
    c.line(cx, 3, cx-9, -3)
    c.line(cx, 3, cx+9, -3)
    c.line(cx-5, -1, cx-12, -6)
    c.line(cx-5, -1, cx-5, -5)
    c.line(cx+5, -1, cx+12, -6)
    c.line(cx+5, -1, cx+5, -5)
    c.line(cx-9, -3, cx-13, -8)
    c.line(cx-9, -3, cx-8, -7)
    c.line(cx+9, -3, cx+13, -8)
    c.line(cx+9, -3, cx+8, -7)
    
    # UPPER H-LINE
    h1y = 13
    hh = 22
    c.hline(cx-hh, cx+hh, h1y)
    
    # RUNES: ᛗ ᛁ ᛗ ᛁ ᚱ  (MIMIR)
    rh = 10
    rt = h1y + 3  # 16
    
    def draw_mannaz(rx):
        c.vline(rx, rt, rt+rh-1)
        c.vline(rx+5, rt, rt+rh-1)
        c.set(rx+1, rt+1)
        c.set(rx+4, rt+1)
        c.set(rx+2, rt+3)
        c.set(rx+3, rt+3)
        c.set(rx+1, rh+rt-2)
        c.set(rx+4, rh+rt-2)
        c.set(rx+2, rh+rt-4)
        c.set(rx+3, rh+rt-4)
    
    def draw_isa(rx):
        c.vline(rx+1, rt, rt+rh-1)
    
    def draw_raidho(rx):
        c.vline(rx, rt, rt+rh-1)
        c.set(rx+1, rt+2)
        c.set(rx+2, rt+1)
        c.set(rx+3, rt+1)
        c.set(rx+4, rt+2)
        c.set(rx+4, rt+3)
        c.set(rx+3, rt+4)
        c.set(rx+2, rt+4)
        c.set(rx+1, rt+4)
    
    draw_mannaz(34)
    draw_isa(41)
    draw_mannaz(45)
    draw_isa(52)
    draw_raidho(56)
    
    # LOWER H-LINE
    h2y = rt + rh + 2  # 28
    c.hline(cx-hh, cx+hh, h2y)
    
    # CONNECTOR
    c.vline(cx, h2y+1, h2y+4)
    
    # DIAMOND KNOT
    dy = h2y + 10
    dh = 5
    dw = 5
    
    for i in range(dh+1):
        w = round(dw * i / dh)
        c.set(cx-w, dy-dh+i)
        c.set(cx+w, dy-dh+i)
    for i in range(dh+1):
        w = round(dw * (dh-i) / dh)
        c.set(cx-w, dy+i)
        c.set(cx+w, dy+i)
    c.hline(cx-dw+1, cx+dw-1, dy)
    
    art = c.to_braille()
    
    # Build output with labels
    lines = art.split('\n')
    
    # Rune labels centered under the art
    # The art is 48 Braille cells wide
    # Rune positions in Braille cells (pixel_x/2):
    # M1: 34/2=17, I1: 41/2=20.5~21, M2: 45/2=22.5~23, I2: 52/2=26, R: 56/2=28
    # Center each label under its rune
    label_line = ' ' * 15 + 'ᛗ' + ' ' * 3 + 'ᛁ' + ' ' * 3 + 'ᛗ' + ' ' * 3 + 'ᛁ' + ' ' * 3 + 'ᚱ'
    name_line = ' ' * 14 + 'M' + ' ' * 5 + 'I' + ' ' * 5 + 'M' + ' ' * 5 + 'I' + ' ' * 5 + 'R'
    
    separator = '─' * 48
    
    output = '\n'.join([
        '',
        art,
        '',
        separator,
        label_line,
        name_line,
        separator,
        '',
    ])
    
    print(output)
    
    # Save to file
    import os
    out_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'logo-braille.txt')
    with open(out_path, 'w', encoding='utf-8') as f:
        f.write(output)
    print(f'Saved to: {out_path}')


if __name__ == '__main__':
    main()
