package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Files the AI builds from (design §4.6), made here so the corpus needs
// no binaries and the expected answers are known exactly. A case's files
// name these, or paths to real files.
var fixtures = map[string]func() ([]byte, error){
	"floorplan.png":     floorPlan,
	"mug.png":           mugPhoto,
	"chair.obj":         chairOBJ,
	"warehouse-map.zip": warehouseMap,
}

// floorPlan is a 6 × 4 m two-room plan at 100 px/m: 0.15 m walls, a
// dividing wall 3.5 m from the left with a door gap at its bottom, an
// entrance in the bottom wall, the overall dimensions written outside it
// and a 1 m scale bar.
func floorPlan() ([]byte, error) {
	const ppm, ox, oy = 100, 150, 150 // pixels per meter; the plan's top-left corner
	img := image.NewRGBA(image.Rect(0, 0, 900, 750))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	wall := func(x0, y0, x1, y1 float64) { // meters from the top-left corner
		r := image.Rect(ox+int(x0*ppm), oy+int(y0*ppm), ox+int(x1*ppm), oy+int(y1*ppm))
		draw.Draw(img, r, image.Black, image.Point{}, draw.Src)
	}
	const t = 0.15
	wall(0, 0, 6, t)         // top
	wall(0, 0, t, 4)         // left
	wall(6-t, 0, 6, 4)       // right
	wall(0, 4-t, 1.0, 4)     // bottom, left of the entrance
	wall(1.9, 4-t, 6, 4)     // bottom, right of it
	wall(3.5, 0, 3.5+t, 3.0) // dividing wall; door gap from 3.0 to the bottom wall
	grey := image.NewUniform(color.Gray{Y: 90})
	line := func(x0, y0, x1, y1 int) {
		draw.Draw(img, image.Rect(x0, y0, x1+1, y1+1), grey, image.Point{}, draw.Src)
	}
	line(ox, oy-40, ox+600, oy-40) // dimension lines
	line(ox-40, oy, ox-40, oy+400)
	label(img, ox+300, oy-70, "6.00 m", 2)
	label(img, ox-100, oy+200, "4.00 m", 2)
	label(img, ox+175, oy+150, "Living room", 2)
	label(img, ox+475, oy+150, "Bedroom", 2)
	line(ox, oy+520, ox+ppm, oy+520) // scale bar
	line(ox, oy+510, ox, oy+530)
	line(ox+ppm, oy+510, ox+ppm, oy+530)
	label(img, ox+ppm/2, oy+550, "1 m", 2)
	return encodePNG(img)
}

// label writes s centered on (x, y), scale times the size of a 7 × 13
// font.
func label(dst draw.Image, x, y int, s string, scale int) {
	face := basicfont.Face7x13
	w := font.MeasureString(face, s).Ceil()
	small := image.NewRGBA(image.Rect(0, 0, w, 13))
	draw.Draw(small, small.Bounds(), image.White, image.Point{}, draw.Src)
	d := font.Drawer{Dst: small, Src: image.Black, Face: face, Dot: fixed.P(0, 10)}
	d.DrawString(s)
	r := image.Rect(x-w*scale/2, y-13*scale/2, x+w*scale-w*scale/2, y+13*scale-13*scale/2)
	xdraw.NearestNeighbor.Scale(dst, r, small, small.Bounds(), draw.Src, nil)
}

// mugPhoto is a drawn "photo" of a blue mug of coffee with a handle on
// its right, standing on a wooden table.
func mugPhoto() ([]byte, error) {
	const size = 640
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	const cx, top, bottom, rx = 320.0, 210.0, 450.0, 105.0 // the mug's axis, rim and base, radius
	const ry = 26.0                                        // the ellipses' half height
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			var c color.RGBA
			if fy < 400 { // wall
				v := uint8(225 - 20*fy/400)
				c = color.RGBA{v, v, v - 8, 255}
			} else { // table, with wood grain
				g := 12 * math.Sin(fy*0.35+math.Sin(fx*0.02)*3)
				c = color.RGBA{uint8(150 + g), uint8(105 + g*0.7), uint8(65 + g*0.4), 255}
			}
			// The shadow under the mug.
			if dx, dy := (fx-cx-20)/(rx+30), (fy-bottom-4)/(ry+6); dx*dx+dy*dy < 1 {
				c = color.RGBA{c.R / 2, c.G / 2, c.B / 2, 255}
			}
			// The handle: a ring on the right of the body.
			if d := math.Hypot(fx-cx-rx, fy-325); fx > cx+rx-5 && d > 42 && d < 70 {
				shade := 0.75 + 0.25*math.Cos((fy-325)/70*math.Pi/2)
				c = color.RGBA{uint8(40 * shade), uint8(90 * shade), uint8(175 * shade), 255}
			}
			// The body: a cylinder, lit from the left.
			u := (fx - cx) / rx
			if math.Abs(u) < 1 && fy > top && fy < bottom+ry*math.Sqrt(1-u*u) {
				shade := 0.55 + 0.45*math.Cos((u+0.35)*math.Pi/2.2)
				c = color.RGBA{uint8(45 * shade), uint8(100 * shade), uint8(190 * shade), 255}
			}
			// The rim and the coffee inside it.
			if dx, dy := (fx-cx)/rx, (fy-top)/ry; dx*dx+dy*dy < 1 {
				c = color.RGBA{70, 125, 210, 255}
				if dx, dy := (fx-cx)/(rx-10), (fy-top-2)/(ry-6); dx*dx+dy*dy < 1 {
					c = color.RGBA{75, 45, 25, 255}
				}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return encodePNG(img)
}

// chairOBJ is a dining chair in millimeters with Y up, as many modelling
// tools export: 450 mm wide and deep, the seat 490 mm up, 900 mm tall.
func chairOBJ() ([]byte, error) {
	var b strings.Builder
	b.WriteString("# A chair in millimeters, Y up.\n")
	n := 0
	box := func(x0, y0, z0, x1, y1, z1 float64) {
		for _, v := range [][3]float64{{x0, y0, z0}, {x1, y0, z0}, {x1, y1, z0}, {x0, y1, z0},
			{x0, y0, z1}, {x1, y0, z1}, {x1, y1, z1}, {x0, y1, z1}} {
			fmt.Fprintf(&b, "v %g %g %g\n", v[0], v[1], v[2])
		}
		for _, f := range [][4]int{{1, 4, 3, 2}, {5, 6, 7, 8}, {1, 2, 6, 5}, {2, 3, 7, 6}, {3, 4, 8, 7}, {4, 1, 5, 8}} {
			fmt.Fprintf(&b, "f %d %d %d %d\n", n+f[0], n+f[1], n+f[2], n+f[3])
		}
		n += 8
	}
	for _, x := range []float64{0, 410} {
		for _, z := range []float64{0, 410} {
			box(x, 0, z, x+40, 450, z+40) // legs
		}
	}
	box(0, 450, 0, 450, 490, 450)   // seat
	box(0, 490, 410, 450, 900, 450) // back
	return []byte(b.String()), nil
}

// warehouseMap is a Nav2 map (YAML and PGM, zipped) of a 20 × 12 m
// warehouse at 0.05 m/px: walls from (-5, -3) to (15, 9) in the map frame,
// a dock door in the bottom wall and three rows of racks, 12 m long and
// 1 m deep, at y = 0, 3 and 6 from x = -1 to 11. Outside the walls is
// unknown.
func warehouseMap() ([]byte, error) {
	const w, h, res, ox, oy = 440, 280, 0.05, -6.0, -4.0
	pix := bytes.Repeat([]byte{205}, w*h) // unknown
	col := func(x float64) int { return int(math.Round((x - ox) / res)) }
	row := func(y float64) int { return h - int(math.Round((y-oy)/res)) } // row of the cell just above y
	fill := func(x0, y0, x1, y1 float64, v byte) {
		for r := row(y1); r < row(y0); r++ {
			for c := col(x0); c < col(x1); c++ {
				pix[r*w+c] = v
			}
		}
	}
	fill(-5, -3, 15, 9, 254) // free floor
	fill(-5, 8.8, 15, 9, 0)  // walls, 0.2 m thick
	fill(-5, -3, 15, -2.8, 0)
	fill(-5, -3, -4.8, 9, 0)
	fill(14.8, -3, 15, 9, 0)
	fill(-2, -3, 0, -2.8, 254) // the dock door
	for _, y := range []float64{0, 3, 6} {
		fill(-1, y-0.5, 11, y+0.5, 0)
	}
	pgm := append([]byte(fmt.Sprintf("P5\n%d %d\n255\n", w, h)), pix...)
	yaml := fmt.Sprintf("image: warehouse.pgm\nresolution: %g\norigin: [%g, %g, 0.0]\nnegate: 0\n"+
		"occupied_thresh: 0.65\nfree_thresh: 0.196\n", res, ox, oy)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct {
		name string
		data []byte
	}{{"warehouse.yaml", []byte(yaml)}, {"warehouse.pgm", pgm}} {
		fw, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
