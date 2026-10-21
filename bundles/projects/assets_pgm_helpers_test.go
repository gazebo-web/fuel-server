package projects

import (
	"bytes"
	"fmt"
	"image"
)

// encodePGM writes an 8-bit binary graymap (for tests).
func encodePGM(img *image.Gray) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "P5\n# made by a test\n%d %d\n255\n", img.Rect.Dx(), img.Rect.Dy())
	b.Write(img.Pix)
	return b.Bytes()
}
