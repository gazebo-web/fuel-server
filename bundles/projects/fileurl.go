package projects

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
)

// Signed file URLs give read-only access to one project revision without
// an Authorization header, for renderers that fetch files themselves:
// gzweb in the browser (which would otherwise send the user's token to
// every host, including public Fuel) and, later, the headless render
// worker (plan §4.5). A token names owner, project and a resolved commit,
// so what it serves never changes; it expires after FileURLTTL.

// FileURLTTL is how long a signed file URL stays valid.
var FileURLTTL = time.Hour

// ErrBadFileToken means a file URL token is malformed, forged or expired.
var ErrBadFileToken = errors.New("invalid or expired file URL")

var (
	fileURLKeyOnce sync.Once
	fileURLKey     []byte
)

// fileURLSecret returns the HMAC key: PROJECT_FILE_URL_SECRET, or a random
// key per process when unset (URLs then don't survive a restart or work
// across replicas).
func fileURLSecret() []byte {
	fileURLKeyOnce.Do(func() {
		if s := os.Getenv("PROJECT_FILE_URL_SECRET"); s != "" {
			fileURLKey = []byte(s)
			return
		}
		fileURLKey = make([]byte, 32)
		if _, err := rand.Read(fileURLKey); err != nil {
			panic(err)
		}
		log.Println("projects: PROJECT_FILE_URL_SECRET unset; using a per-process key for signed file URLs")
	})
	return fileURLKey
}

func signFileToken(owner, name, rev string, expires time.Time) string {
	payload := strings.Join([]string{owner, name, rev, strconv.FormatInt(expires.Unix(), 10)}, "\n")
	mac := hmac.New(sha256.New, fileURLSecret())
	mac.Write([]byte(payload))
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(payload)) + "." + enc.EncodeToString(mac.Sum(nil))
}

// verifyFileToken returns the owner, project name and commit a token grants.
func verifyFileToken(token string, now time.Time) (owner, name, rev string, err error) {
	enc := base64.RawURLEncoding
	p, s, ok := strings.Cut(token, ".")
	if !ok {
		return "", "", "", ErrBadFileToken
	}
	payload, err1 := enc.DecodeString(p)
	sig, err2 := enc.DecodeString(s)
	if err1 != nil || err2 != nil {
		return "", "", "", ErrBadFileToken
	}
	mac := hmac.New(sha256.New, fileURLSecret())
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", "", "", ErrBadFileToken
	}
	parts := strings.Split(string(payload), "\n")
	if len(parts) != 4 {
		return "", "", "", ErrBadFileToken
	}
	exp, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || now.Unix() > exp {
		return "", "", "", ErrBadFileToken
	}
	return parts[0], parts[1], parts[2], nil
}

// FileURL is a signed, read-only base path for one project revision.
type FileURL struct {
	// BasePath is relative to the API root, e.g. /1.0/project-files/<token>.
	// Files are at BasePath/files/<path>; a project model is also exposed
	// like a Fuel model at BasePath/models/<model> (…/tip/files lists it).
	BasePath  string    `json:"base_path"`
	Rev       string    `json:"rev"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SignFileURL issues a FileURL for rev ("" or "tip" = HEAD) of a project
// the caller can read.
func (s *Service) SignFileURL(tx *gorm.DB, name, rev string, jwtUser *users.User) (*FileURL, *gz.ErrMsg) {
	p, em := s.GetForCaller(tx, name, jwtUser)
	if em != nil {
		return nil, em
	}
	_, resolved, err := listFilesAt(*p.Location, rev)
	if err != nil {
		return nil, repoErr(err)
	}
	exp := time.Now().Add(FileURLTTL).Truncate(time.Second)
	return &FileURL{
		BasePath:  "/" + apiVersion + "/project-files/" + signFileToken(*p.Owner, *p.Name, resolved, exp),
		Rev:       resolved,
		ExpiresAt: exp,
	}, nil
}

const apiVersion = "1.0"

// FileByToken reads relPath at the revision a signed token grants.
func (s *Service) FileByToken(ctx context.Context, tx *gorm.DB, token, relPath string) ([]byte, *gz.ErrMsg) {
	p, rev, em := projectForToken(tx, token)
	if em != nil {
		return nil, em
	}
	if rev == assetsRev {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnauthorized, ErrBadFileToken)
	}
	b, _, err := readFileAt(*p.Location, relPath, rev)
	if err != nil {
		return nil, repoErr(err)
	}
	return b, nil
}

// FilesByToken lists the files under dir at the revision a token grants,
// relative to dir.
func (s *Service) FilesByToken(tx *gorm.DB, token, dir string) ([]string, *gz.ErrMsg) {
	p, rev, em := projectForToken(tx, token)
	if em != nil {
		return nil, em
	}
	if rev == assetsRev {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnauthorized, ErrBadFileToken)
	}
	files, _, err := listFilesAt(*p.Location, rev)
	if err != nil {
		return nil, repoErr(err)
	}
	prefix := strings.TrimSuffix(dir, "/") + "/"
	var out []string
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			out = append(out, strings.TrimPrefix(f, prefix))
		}
	}
	if len(out) == 0 {
		return nil, repoErr(fmt.Errorf("%w: %s", ErrFileNotFound, dir))
	}
	return out, nil
}

// assetsRev is the "revision" of a token for a project's uploaded files
// (assets.go) rather than a commit of its repo.
const assetsRev = "@assets"

// SignAssetsURL issues a signed base path for a project's uploaded files:
// BasePath/<asset id>/<file> serves its original or a view, so <img> tags
// can show them.
func (s *Service) SignAssetsURL(tx *gorm.DB, name string, jwtUser *users.User) (*FileURL, *gz.ErrMsg) {
	p, em := s.GetForCaller(tx, name, jwtUser)
	if em != nil {
		return nil, em
	}
	exp := time.Now().Add(FileURLTTL).Truncate(time.Second)
	return &FileURL{
		BasePath:  "/" + apiVersion + "/project-assets/" + signFileToken(*p.Owner, *p.Name, assetsRev, exp),
		ExpiresAt: exp,
	}, nil
}

// AssetFileByToken is the path of an uploaded file's original or view
// through a token from SignAssetsURL.
func (s *Service) AssetFileByToken(tx *gorm.DB, token, ref, file string) (*Asset, string, *gz.ErrMsg) {
	p, rev, em := projectForToken(tx, token)
	if em != nil {
		return nil, "", em
	}
	if rev != assetsRev {
		return nil, "", gz.NewErrorMessageWithBase(gz.ErrorUnauthorized, ErrBadFileToken)
	}
	a, em := findAsset(tx, p, ref)
	if em != nil {
		return nil, "", em
	}
	return assetFilePath(p, a, file)
}

func projectForToken(tx *gorm.DB, token string) (*Project, string, *gz.ErrMsg) {
	owner, name, rev, err := verifyFileToken(token, time.Now())
	if err != nil {
		em := gz.NewErrorMessageWithBase(gz.ErrorUnauthorized, err)
		return nil, "", em
	}
	p, err := GetProjectByName(tx, name, owner)
	if err != nil || p == nil {
		return nil, "", gz.NewErrorMessage(gz.ErrorNameNotFound)
	}
	return p, rev, nil
}
