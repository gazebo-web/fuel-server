package projects

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// Asset kinds.
const (
	AssetImage    = "image"    // a photo, sketch or reference picture
	AssetDocument = "document" // a PDF
	AssetDrawing  = "drawing"  // a 2D vector drawing: SVG or DXF
	AssetMesh     = "mesh"     // a 3D mesh, or a zip of one with its textures
	AssetTexture  = "texture"  // an image, or a zip of a PBR texture set
)

// supportedTypes names what can be uploaded.
const supportedTypes = "PNG, JPEG, WebP, PDF, SVG, DXF, GLB, OBJ, STL or DAE, " +
	"or a zip of any of these, texture sets and Gazebo model folders"

var meshExts = map[string]bool{".glb": true, ".gltf": true, ".obj": true, ".stl": true, ".dae": true}
var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}

// sniffAsset decides an upload's kind and media type from its content,
// never its name. A zip holding one mesh (plus what it uses) is a mesh,
// one of only images a texture set; any other zip is a bundle, unpacked
// into files of their own (assets_bundle.go).
func sniffAsset(data []byte) (kind, mime string, info AssetInfo, err error) {
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}
	switch {
	case len(data) == 0:
		return "", "", nil, fmt.Errorf("the file is empty")
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return AssetImage, "image/png", nil, nil
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return AssetImage, "image/jpeg", nil, nil
	case len(data) > 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return AssetImage, "image/webp", nil, nil
	case len(data) > 3 && (string(data[:2]) == "P5" || string(data[:2]) == "P2") && isSpace(data[2]):
		return AssetImage, pgmMime, nil, nil
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return AssetDocument, "application/pdf", nil, nil
	case bytes.HasPrefix(data, []byte("glTF")):
		return AssetMesh, "model/gltf-binary", nil, nil
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		return sniffZip(data)
	case isBinarySTL(data):
		return AssetMesh, "model/stl", nil, nil
	case bytes.IndexByte(head, 0) >= 0 || !validUTF8Prefix(head):
		return "", "", nil, fmt.Errorf("unsupported file type; upload %s", supportedTypes)
	}
	text := strings.ToLower(string(head))
	trimmed := strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	switch {
	case strings.HasPrefix(trimmed, "solid") && strings.Contains(text, "facet"):
		return AssetMesh, "model/stl", nil, nil
	case strings.HasPrefix(trimmed, "<") && strings.Contains(text, "<collada"):
		return AssetMesh, "model/vnd.collada+xml", nil, nil
	case strings.HasPrefix(trimmed, "<") && strings.Contains(text, "<svg"):
		return AssetDrawing, "image/svg+xml", nil, nil
	case isDXF(trimmed):
		return AssetDrawing, "image/vnd.dxf", nil, nil
	case isOBJ(text):
		return AssetMesh, "model/obj", nil, nil
	}
	if _, ok := parseNavMap(data); ok {
		return "", "", nil, fmt.Errorf("this is a map's YAML: upload it in one zip with its image")
	}
	return "", "", nil, fmt.Errorf("unsupported file type; upload %s", supportedTypes)
}

// validUTF8Prefix reports whether b, a prefix of a file, is UTF-8 text
// apart from a character cut off at its end.
func validUTF8Prefix(b []byte) bool {
	for i := 0; i < utf8.UTFMax && len(b) > 0; i++ {
		if utf8.Valid(b) {
			return true
		}
		b = b[:len(b)-1]
	}
	return len(b) == 0
}

// isBinarySTL: an 80-byte header, a triangle count, 50 bytes per triangle.
// pgmMime is a portable graymap's type (ROS occupancy map images).
const pgmMime = "image/x-portable-graymap"

func isSpace(b byte) bool { return b == ' ' || b == '\n' || b == '\r' || b == '\t' }

func isBinarySTL(data []byte) bool {
	if len(data) < 84 {
		return false
	}
	n := binary.LittleEndian.Uint32(data[80:84])
	return n > 0 && 84+50*uint64(n) == uint64(len(data))
}

func zipHas(zr *zip.Reader, name string) bool {
	for _, f := range zr.File {
		if f.Name == name {
			return true
		}
	}
	return false
}

// isDXF: DXF files are group-code/value line pairs, starting "0 SECTION"
// (after any 999 comment pairs).
func isDXF(text string) bool {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i+1 < len(lines); i += 2 {
		code, val := strings.TrimSpace(lines[i]), strings.TrimSpace(lines[i+1])
		if code == "999" {
			continue
		}
		return code == "0" && val == "section"
	}
	return false
}

// isOBJ: vertex and face lines.
func isOBJ(text string) bool {
	var v, f bool
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		v = v || strings.HasPrefix(line, "v ")
		f = f || strings.HasPrefix(line, "f ")
	}
	return v && f
}

// sniffZip checks a zip's entries (count, unpacked size, safe names) and
// whether it is a mesh with its files, a texture set, or a bundle to
// unpack. Its info lists the files and, for a mesh, the main one.
func sniffZip(data []byte) (kind, mime string, info AssetInfo, err error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", "", nil, fmt.Errorf("unreadable zip: %v", err)
	}
	var files, meshes []string
	images := 0
	hasModelConfig := false
	var mapYAMLs []string // map YAMLs whose image is in the zip
	var total uint64
	for _, f := range zr.File {
		name := f.Name
		if f.FileInfo().IsDir() || strings.HasPrefix(name, "__MACOSX/") || strings.HasPrefix(path.Base(name), ".") {
			continue
		}
		if strings.Contains(name, "\\") || path.IsAbs(name) || path.Clean(name) != name ||
			name == ".." || strings.HasPrefix(name, "../") {
			return "", "", nil, fmt.Errorf("the zip has an unsafe file name %q", name)
		}
		if len(files) == MaxAssetZipEntries {
			return "", "", nil, fmt.Errorf("the zip has more than %d files", MaxAssetZipEntries)
		}
		total += f.UncompressedSize64
		if total > MaxAssetZipUnpacked {
			return "", "", nil, fmt.Errorf("the zip unpacks to more than %d MB", MaxAssetZipUnpacked>>20)
		}
		files = append(files, name)
		hasModelConfig = hasModelConfig || path.Base(name) == "model.config"
		if isYAML(name) && f.UncompressedSize64 <= maxMapYAML {
			if rc, err := f.Open(); err == nil {
				b, _ := io.ReadAll(io.LimitReader(rc, maxMapYAML+1))
				rc.Close()
				if m, ok := parseNavMap(b); ok {
					if _, ok := mapImage(name, m, func(n string) bool { return zipHas(zr, n) }); ok {
						mapYAMLs = append(mapYAMLs, name)
					}
				}
			}
		}
		ext := strings.ToLower(path.Ext(name))
		switch {
		case meshExts[ext]:
			meshes = append(meshes, name)
		case imageExts[ext]:
			images++
		}
	}
	switch {
	case len(mapYAMLs) == 1 && len(files) == 2:
		// A map: its YAML and its image.
		return AssetMap, "application/zip", AssetInfo{"files": files, "main": mapYAMLs[0]}, nil
	case hasModelConfig:
		// Gazebo models (and maybe more): unpacked by expandBundle.
		return assetBundle, "application/zip", AssetInfo{"files": files}, nil
	case len(meshes) == 1:
		return AssetMesh, "application/zip", AssetInfo{"files": files, "main": meshes[0]}, nil
	case images > 0 && images == len(files):
		return AssetTexture, "application/zip", AssetInfo{"files": files}, nil
	case len(files) == 0:
		return "", "", nil, fmt.Errorf("the zip is empty")
	}
	// Several meshes, or a mix of files: unpacked into files of their own.
	return assetBundle, "application/zip", AssetInfo{"files": files}, nil
}
