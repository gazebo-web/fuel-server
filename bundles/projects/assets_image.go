package projects

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // decode PNG uploads
	"strconv"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp" // decode WebP uploads
)

// Image uploads keep their original pixels, minus metadata (EXIF and XMP
// carry camera location), and get two derived views: ai.jpg, the copy
// models look at, and thumb.jpg for the file list.
const (
	// AIViewSize is the long side of ai.jpg: the largest image Anthropic
	// uses without downscaling it itself; other providers accept it too.
	AIViewSize = 1568
	thumbSize  = 256
)

// maxDecodeBytes caps the memory one decoded image may take (pixels
// times bytes per pixel): a 50 MP 16-bit PNG would be 400 MB, and
// compresses to almost nothing.
const maxDecodeBytes = 256 << 20

// decodeSlots bounds how many large images decode at once.
var decodeSlots = make(chan struct{}, 2)

// DecodeImage decodes an upload within the pixel and memory limits,
// waiting for a decode slot.
func DecodeImage(data []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("unreadable image: %v", err)
	}
	px := int64(cfg.Width) * int64(cfg.Height)
	if px > MaxAssetPixels {
		return nil, fmt.Errorf("the image is %d×%d; the limit is %d megapixels",
			cfg.Width, cfg.Height, MaxAssetPixels/1_000_000)
	}
	bpp := int64(4)
	switch cfg.ColorModel {
	case color.RGBA64Model, color.NRGBA64Model:
		bpp = 8
	case color.Gray16Model:
		bpp = 2
	}
	if px*bpp > maxDecodeBytes {
		return nil, fmt.Errorf("the image is %d×%d at 16 bits a channel, too large to read; save it at 8 bits",
			cfg.Width, cfg.Height)
	}
	decodeSlots <- struct{}{}
	defer func() { <-decodeSlots }()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("unreadable image: %v", err)
	}
	return img, nil
}

// processImage returns the cleaned original, the derived views and info.
// A JPEG's orientation stays a tag on the original (in a new, minimal
// EXIF segment) and is applied to the views, which are small.
func processImage(data []byte, mime string) (clean []byte, views map[string][]byte, info AssetInfo, err error) {
	img, err := DecodeImage(data)
	if err != nil {
		return nil, nil, nil, err
	}
	orientation := 1
	switch mime {
	case "image/jpeg":
		orientation = jpegOrientation(data)
		clean, err = stripJPEGMetadata(data, orientation)
	case "image/png":
		clean, err = stripPNGMetadata(data)
	case "image/webp":
		clean, err = stripWebPMetadata(data)
	default:
		err = fmt.Errorf("unsupported image type %s", mime)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	ai, err := encodeJPEG(flatten(orient(fit(img, AIViewSize), orientation)), 85)
	if err != nil {
		return nil, nil, nil, err
	}
	thumb, err := encodeJPEG(flatten(orient(fit(img, thumbSize), orientation)), 80)
	if err != nil {
		return nil, nil, nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if orientation >= 5 {
		w, h = h, w
	}
	info = AssetInfo{"width": w, "height": h}
	return clean, map[string][]byte{"ai.jpg": ai, "thumb.jpg": thumb}, info, nil
}

// fit scales img down so its long side is at most size.
func fit(img image.Image, size int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= size && h <= size {
		return img
	}
	if w >= h {
		h, w = max(1, h*size/w), size
	} else {
		w, h = max(1, w*size/h), size
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// flatten puts img on white, for JPEG.
func flatten(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Over)
	return dst
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// orient applies an EXIF orientation (2-8) to img; 1 returns it as is.
func orient(img image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if o >= 5 {
		w, h = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = b.Dx()-1-x, y
			case 3:
				dx, dy = b.Dx()-1-x, b.Dy()-1-y
			case 4:
				dx, dy = x, b.Dy()-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = b.Dy()-1-y, x
			case 7:
				dx, dy = b.Dy()-1-y, b.Dx()-1-x
			case 8:
				dx, dy = y, b.Dx()-1-x
			default:
				dx, dy = x, y
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// jpegOrientation reads the EXIF orientation tag (1 when absent).
func jpegOrientation(data []byte) int {
	for i := 2; i+4 <= len(data) && data[i] == 0xff; {
		marker := data[i+1]
		if marker == 0xda || marker == 0xd9 { // start of scan, end of image
			break
		}
		n := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if i+2+n > len(data) {
			break
		}
		seg := data[i+4 : i+2+n]
		if marker == 0xe1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			if o := tiffOrientation(seg[6:]); o >= 1 && o <= 8 {
				return o
			}
		}
		i += 2 + n
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	ifd := int(bo.Uint32(t[4:8]))
	if ifd+2 > len(t) {
		return 0
	}
	count := int(bo.Uint16(t[ifd : ifd+2]))
	for j := 0; j < count; j++ {
		e := ifd + 2 + 12*j
		if e+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[e:e+2]) == 0x0112 {
			return int(bo.Uint16(t[e+8 : e+10]))
		}
	}
	return 0
}

// stripJPEGMetadata keeps a JPEG's image and color data and drops the
// rest, without re-encoding:
//   - kept: JFIF (APP0), ICC color profiles (APP2 "ICC_PROFILE"), Adobe
//     color info (APP14), tables, frames and scans;
//   - dropped: EXIF and XMP (camera, time, location), other APP segments,
//     comments, and anything after the end of the image, where
//     Multi-Picture files carry more images with their own EXIF.
//
// An orientation other than 1 is written back as the only EXIF tag, so
// the photo still shows upright.
func stripJPEGMetadata(data []byte, orientation int) ([]byte, error) {
	bad := fmt.Errorf("unreadable JPEG")
	out := bytes.NewBuffer(make([]byte, 0, len(data)))
	out.Write(data[:2])
	if orientation >= 2 && orientation <= 8 {
		out.Write(orientationEXIF(orientation))
	}
	keep := func(marker byte, seg []byte) bool {
		switch {
		case marker == 0xe2:
			return bytes.HasPrefix(seg, []byte("ICC_PROFILE\x00"))
		case marker == 0xe0 || marker == 0xee:
			return true
		case marker >= 0xe1 && marker <= 0xef, marker == 0xfe:
			return false
		}
		return true
	}
	i := 2
	for i+2 <= len(data) {
		if data[i] != 0xff {
			return nil, bad
		}
		marker := data[i+1]
		if marker == 0xff { // fill byte
			i++
			continue
		}
		if marker == 0xd9 { // end of image: stop, dropping what follows
			out.Write(data[i : i+2])
			return out.Bytes(), nil
		}
		if i+4 > len(data) {
			return nil, bad
		}
		n := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if n < 2 || i+2+n > len(data) {
			return nil, bad
		}
		if keep(marker, data[i+4:i+2+n]) {
			out.Write(data[i : i+2+n])
		}
		i += 2 + n
		if marker != 0xda {
			continue
		}
		// A scan's entropy-coded data runs to the next marker that isn't a
		// stuffed 0xff00 or a restart marker.
		start := i
		for i+1 < len(data) && !(data[i] == 0xff && data[i+1] != 0 && (data[i+1] < 0xd0 || data[i+1] > 0xd7)) {
			i++
		}
		out.Write(data[start:i])
	}
	return nil, bad
}

// orientationEXIF is an APP1 segment holding only an orientation tag.
func orientationEXIF(o int) []byte {
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08")
	tiff = binary.BigEndian.AppendUint16(tiff, 1)                       // one entry
	tiff = append(tiff, 0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01) // orientation, SHORT, 1 value
	tiff = binary.BigEndian.AppendUint16(tiff, uint16(o))
	tiff = append(tiff, 0, 0, 0, 0, 0, 0) // padding, then no next IFD
	seg := append([]byte("Exif\x00\x00"), tiff...)
	out := []byte{0xff, 0xe1}
	out = binary.BigEndian.AppendUint16(out, uint16(len(seg)+2))
	return append(out, seg...)
}

// stripPNGMetadata drops eXIf, text and time chunks.
func stripPNGMetadata(data []byte) ([]byte, error) {
	drop := map[string]bool{"eXIf": true, "tEXt": true, "zTXt": true, "iTXt": true, "tIME": true}
	out := bytes.NewBuffer(make([]byte, 0, len(data)))
	out.Write(data[:8])
	for i := 8; i < len(data); {
		if i+12 > len(data) {
			return nil, fmt.Errorf("unreadable PNG")
		}
		n := int(binary.BigEndian.Uint32(data[i : i+4]))
		end := i + 12 + n
		if n < 0 || end > len(data) {
			return nil, fmt.Errorf("unreadable PNG")
		}
		typ := string(data[i+4 : i+8])
		if !drop[typ] {
			out.Write(data[i:end])
		}
		if typ == "IEND" {
			break // some tools append bytes after the image; drop them
		}
		i = end
	}
	return out.Bytes(), nil
}

// stripWebPMetadata drops the EXIF and XMP chunks and clears their flags.
func stripWebPMetadata(data []byte) ([]byte, error) {
	var body bytes.Buffer
	for i := 12; i < len(data); {
		if i+8 > len(data) {
			return nil, fmt.Errorf("unreadable WebP")
		}
		id := string(data[i : i+4])
		n := int(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		end := i + 8 + n + n%2
		if n < 0 || i+8+n > len(data) {
			return nil, fmt.Errorf("unreadable WebP")
		}
		end = min(end, len(data))
		switch id {
		case "EXIF", "XMP ":
		case "VP8X":
			chunk := append([]byte(nil), data[i:end]...)
			if len(chunk) > 8 {
				chunk[8] &^= 0x08 | 0x04 // EXIF and XMP present
			}
			body.Write(chunk)
		default:
			body.Write(data[i:end])
		}
		i = end
	}
	out := make([]byte, 12, 12+body.Len())
	copy(out, data[:12])
	binary.LittleEndian.PutUint32(out[4:8], uint32(4+body.Len()))
	return append(out, body.Bytes()...), nil
}

// GridOverlay draws labelled lines every tenth of the width and height on
// a JPEG, so a model can read positions off a drawing ("the wall runs
// from 20% to 70% across").
func GridOverlay(jpg []byte) ([]byte, error) {
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpg)); err != nil {
		return nil, err
	} else if cfg.Width > 2*AIViewSize || cfg.Height > 2*AIViewSize {
		return nil, fmt.Errorf("the picture is %d×%d, larger than a view", cfg.Width, cfg.Height)
	}
	src, err := jpeg.Decode(bytes.NewReader(jpg))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	img := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(img, img.Bounds(), src, b.Min, draw.Src)
	line := color.RGBA{230, 0, 90, 255}
	w, h := b.Dx(), b.Dy()
	for k := 0; k <= 10; k++ {
		x, y := min(w-1, k*w/10), min(h-1, k*h/10)
		for yy := 0; yy < h; yy++ {
			blend(img, x, yy, line)
		}
		for xx := 0; xx < w; xx++ {
			blend(img, xx, y, line)
		}
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(line), Face: basicfont.Face7x13}
	label := func(s string, x, y int) {
		x = max(1, min(x, w-d.MeasureString(s).Ceil()-1))
		y = max(12, min(y, h-2))
		// A white box keeps labels readable on busy drawings.
		box := image.Rect(x-1, y-11, x+d.MeasureString(s).Ceil()+1, y+2)
		draw.Draw(img, box, image.White, image.Point{}, draw.Src)
		d.Dot = fixed.P(x, y)
		d.DrawString(s)
	}
	for k := 1; k < 10; k++ {
		label(strconv.Itoa(k*10)+"%", k*w/10+2, 12)
		label(strconv.Itoa(k*10)+"%", 2, k*h/10-2)
	}
	return encodeJPEG(img, 85)
}

// blend draws a half-transparent pixel.
func blend(img *image.RGBA, x, y int, c color.RGBA) {
	o := img.RGBAAt(x, y)
	mix := func(a, b uint8) uint8 { return uint8((int(a) + int(b)) / 2) }
	img.SetRGBA(x, y, color.RGBA{mix(o.R, c.R), mix(o.G, c.G), mix(o.B, c.B), 255})
}
