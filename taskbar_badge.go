package main

import "strconv"

const taskbarBadgeSize = 32

var badgeGlyphs = map[rune][]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "010", "010", "010"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
	'+': {"000", "010", "111", "010", "000"},
}

func taskbarBadgeLabel(count int) string {
	if count <= 0 {
		return ""
	}
	if count > 99 {
		return "99+"
	}
	return strconv.Itoa(count)
}

// renderTaskbarBadge produces the AND/XOR masks consumed by CreateIcon. The
// bitmap rows are bottom-up, as required by the Win32 icon APIs.
func renderTaskbarBadge(count int) ([]byte, []byte) {
	label := taskbarBadgeLabel(count)
	andMask := make([]byte, taskbarBadgeSize*taskbarBadgeSize/8)
	for index := range andMask {
		andMask[index] = 0xff
	}
	xorMask := make([]byte, taskbarBadgeSize*taskbarBadgeSize*4)

	setPixel := func(x, y int, blue, green, red, alpha byte) {
		if x < 0 || x >= taskbarBadgeSize || y < 0 || y >= taskbarBadgeSize {
			return
		}
		storageY := taskbarBadgeSize - 1 - y
		andIndex := storageY*(taskbarBadgeSize/8) + x/8
		andMask[andIndex] &^= byte(0x80 >> uint(x%8))
		offset := (storageY*taskbarBadgeSize + x) * 4
		xorMask[offset], xorMask[offset+1], xorMask[offset+2], xorMask[offset+3] = blue, green, red, alpha
	}

	// Red notification circle with a subtle darker edge.
	for y := 0; y < taskbarBadgeSize; y++ {
		for x := 0; x < taskbarBadgeSize; x++ {
			dx, dy := float64(x)-15.5, float64(y)-15.5
			distanceSquared := dx*dx + dy*dy
			if distanceSquared <= 15.25*15.25 {
				if distanceSquared > 13.75*13.75 {
					setPixel(x, y, 38, 38, 190, 255)
				} else {
					setPixel(x, y, 48, 55, 239, 255)
				}
			}
		}
	}

	if label == "" {
		return andMask, xorMask
	}
	scale := 4
	if len(label) == 2 {
		scale = 3
	} else if len(label) >= 3 {
		scale = 2
	}
	width := (len(label)*3 + len(label) - 1) * scale
	height := 5 * scale
	startX, startY := (taskbarBadgeSize-width)/2, (taskbarBadgeSize-height)/2
	for glyphIndex, character := range label {
		glyph := badgeGlyphs[character]
		for row, pixels := range glyph {
			for column, pixel := range pixels {
				if pixel != '1' {
					continue
				}
				for offsetY := 0; offsetY < scale; offsetY++ {
					for offsetX := 0; offsetX < scale; offsetX++ {
						x := startX + glyphIndex*4*scale + column*scale + offsetX
						y := startY + row*scale + offsetY
						setPixel(x, y, 255, 255, 255, 255)
					}
				}
			}
		}
	}
	return andMask, xorMask
}
