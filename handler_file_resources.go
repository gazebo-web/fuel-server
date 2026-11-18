// handler_file_resources provides common handlers to deal with Fuel's file based
// resources (eg. models, worlds, collections).

package main

import (
	"context"
	"fmt"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type getFileService interface {
	GetFile(ctx context.Context, tx *gorm.DB, owner, name, path, version string,
		user *users.User) (io.ReadSeekCloser, int, *gz.ErrMsg)
}

// IndividualFileDownload is used to download a single file from a service
// that implements the GetFileService interface.
func IndividualFileDownload(s getFileService, owner, name string, jwtUser *users.User,
	tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	// Extract the path from the request.
	path := mux.Vars(r)["path"]
	cleanPath := filepath.Clean(path)
	// Get the version
	version, valid := mux.Vars(r)["version"]
	// If version does not exist
	if !valid {
		return nil, gz.NewErrorMessage(gz.ErrorVersionNotFound)
	}

	// Remove request header to always serve fresh
	r.Header.Del("If-Modified-Since")
	// Also tag it as "attachment" to force a file download
	filename := filepath.Base(cleanPath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	content, ver, em := s.GetFile(r.Context(), tx, owner, name, cleanPath, version, jwtUser)
	if em != nil {
		return nil, em
	}
	defer content.Close()

	writeIgnResourceVersionHeader(w, ver)

	// The content is read from the repository as it is sent, and it can only
	// be read forward: each range of a multi-range request that starts before
	// the previous one would read the file again from its beginning. Send the
	// whole file instead, as RFC 7233 allows.
	if strings.Contains(r.Header.Get("Range"), ",") {
		r.Header.Del("Range")
	}

	modtime := time.Now()
	// Note: ServeContent should be always last line, after all headers were set.
	http.ServeContent(w, r, filename, modtime, content)
	return nil, nil
}
