package projects

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"io"
	"strconv"
)

// PGM (portable graymap), the image format of ROS/Nav2 occupancy maps:
// binary (P5, 8 or 16 bits) and plain text (P2). Registered with the
// image package so DecodeImage and DecodeConfig read it like PNG.

func init() {
	image.RegisterFormat("pgm", "P5", decodePGM, decodePGMConfig)
	image.RegisterFormat("pgm", "P2", decodePGM, decodePGMConfig)
}

type pgmHeader struct {
	plain         bool
	width, height int
	maxVal        int
}

func readPGMHeader(r *bufio.Reader) (pgmHeader, error) {
	var h pgmHeader
	magic, err := pgmToken(r)
	if err != nil {
		return h, err
	}
	switch magic {
	case "P5":
	case "P2":
		h.plain = true
	default:
		return h, fmt.Errorf("pgm: not a graymap (%q)", magic)
	}
	var nums [3]int
	for i := range nums {
		tok, err := pgmToken(r)
		if err != nil {
			return h, err
		}
		if nums[i], err = strconv.Atoi(tok); err != nil || nums[i] <= 0 {
			return h, fmt.Errorf("pgm: bad header value %q", tok)
		}
	}
	h.width, h.height, h.maxVal = nums[0], nums[1], nums[2]
	if h.maxVal > 65535 {
		return h, fmt.Errorf("pgm: maxval %d", h.maxVal)
	}
	return h, nil
}

// pgmToken reads the next whitespace-separated header token, skipping #
// comments. For P5 exactly one whitespace byte follows the last one.
func pgmToken(r *bufio.Reader) (string, error) {
	var tok []byte
	for {
		c, err := r.ReadByte()
		if err != nil {
			if err == io.EOF && len(tok) > 0 {
				return string(tok), nil
			}
			return "", fmt.Errorf("pgm: truncated header")
		}
		switch {
		case c == '#' && len(tok) == 0:
			if _, err := r.ReadBytes('\n'); err != nil {
				return "", fmt.Errorf("pgm: truncated header")
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if len(tok) > 0 {
				return string(tok), nil
			}
		default:
			if len(tok) > 16 {
				return "", fmt.Errorf("pgm: bad header")
			}
			tok = append(tok, c)
		}
	}
}

func decodePGMConfig(r io.Reader) (image.Config, error) {
	h, err := readPGMHeader(bufio.NewReader(r))
	if err != nil {
		return image.Config{}, err
	}
	model := color.GrayModel
	if h.maxVal > 255 {
		model = color.Gray16Model
	}
	return image.Config{ColorModel: model, Width: h.width, Height: h.height}, nil
}

// decodePGM returns an 8-bit image, values scaled to 0-255.
func decodePGM(r io.Reader) (image.Image, error) {
	br := bufio.NewReader(r)
	h, err := readPGMHeader(br)
	if err != nil {
		return nil, err
	}
	if int64(h.width)*int64(h.height) > MaxAssetPixels {
		return nil, fmt.Errorf("pgm: %d×%d is too large", h.width, h.height)
	}
	img := image.NewGray(image.Rect(0, 0, h.width, h.height))
	scale := func(v int) uint8 {
		if v > h.maxVal {
			v = h.maxVal
		}
		return uint8(v * 255 / h.maxVal)
	}
	n := h.width * h.height
	switch {
	case h.plain:
		for i := 0; i < n; i++ {
			tok, err := pgmToken(br)
			if err != nil {
				return nil, fmt.Errorf("pgm: truncated pixels")
			}
			v, err := strconv.Atoi(tok)
			if err != nil {
				return nil, fmt.Errorf("pgm: bad pixel %q", tok)
			}
			img.Pix[i] = scale(v)
		}
	case h.maxVal > 255:
		buf := make([]byte, 2*n)
		if _, err := io.ReadFull(br, buf); err != nil {
			return nil, fmt.Errorf("pgm: truncated pixels")
		}
		for i := 0; i < n; i++ {
			img.Pix[i] = scale(int(buf[2*i])<<8 | int(buf[2*i+1]))
		}
	default:
		if _, err := io.ReadFull(br, img.Pix); err != nil {
			return nil, fmt.Errorf("pgm: truncated pixels")
		}
		if h.maxVal != 255 {
			for i, v := range img.Pix {
				img.Pix[i] = scale(int(v))
			}
		}
	}
	return img, nil
}
