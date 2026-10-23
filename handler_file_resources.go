// handler_file_resources provides common handlers to deal with Fuel's file based
// resources (eg. models, worlds, collections).

package main

import (
	"context"
	"fmt"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/vcs"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type getFileService interface {
	GetFile(ctx context.Context, tx *gorm.DB, owner, name, path, version string,
		user *users.User) (*vcs.File, int, *gz.ErrMsg)
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
	// The file content is read while it is sent, and each range of a multi-range
	// request that starts before the previous one reads the file again from its
	// beginning. Serve the whole file instead, as RFC 7233 allows.
	if strings.Contains(r.Header.Get("Range"), ",") {
		r.Header.Del("Range")
	}
	// Also tag it as "attachment" to force a file download
	filename := filepath.Base(cleanPath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	f, ver, em := s.GetFile(r.Context(), tx, owner, name, cleanPath, version, jwtUser)
	if em != nil {
		return nil, em
	}

	writeIgnResourceVersionHeader(w, ver)

	// The content is read from the repository while it is sent, so that large
	// files are not loaded in memory.
	content, err := f.Content()
	if err != nil {
		if r.Context().Err() == nil {
			gz.LoggerFromContext(r.Context()).Error(fmt.Sprintf("Error while reading file %s of %s/%s",
				cleanPath, owner, name), err)
		}
		return nil, gz.NewErrorMessageWithBase(gz.ErrorRepo, err)
	}
	defer content.Close()

	modtime := time.Now()
	// Note: ServeContent should be called after all headers were set.
	http.ServeContent(w, r, filename, modtime, content)
	// A read error after the response started is not reported to the client,
	// other than by a response shorter than its Content-Length.
	if err := content.Err(); err != nil && r.Context().Err() == nil {
		gz.LoggerFromContext(r.Context()).Error(fmt.Sprintf("Error while sending file %s of %s/%s",
			cleanPath, owner, name), err)
	}
	return nil, nil
}
