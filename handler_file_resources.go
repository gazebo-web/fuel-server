// handler_file_resources provides common handlers to deal with Fuel's file based
// resources (eg. models, worlds, collections).

package main

import (
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"bytes"
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"net/http"
	"path/filepath"
	"strconv"
	"time"
)

type getFileService interface {
	GetFile(ctx context.Context, tx *gorm.DB, owner, name, path, version string,
		user *users.User) (*[]byte, int, *ign.ErrMsg)
}

// IndividualFileDownload is used to download a single file from a service
// that implements the GetFileService interface.
func IndividualFileDownload(s getFileService, owner, name string, jwtUser *users.User,
	tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *ign.ErrMsg) {

	// Extract the path from the request.
	path := mux.Vars(r)["path"]
	cleanPath := filepath.Clean(path)
	// Get the version
	version, valid := mux.Vars(r)["version"]
	// If version does not exist
	if !valid {
		return nil, ign.NewErrorMessage(ign.ErrorVersionNotFound)
	}

	// Remove request header to always serve fresh
	r.Header.Del("If-Modified-Since")
	// Also tag it as "attachment" to force a file download
	filename := filepath.Base(cleanPath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	bs, ver, em := s.GetFile(r.Context(), tx, owner, name, cleanPath, version, jwtUser)
	if em != nil {
		return nil, em
	}

	writeIgnResourceVersionHeader(strconv.Itoa(ver), w, r)

	modtime := time.Now()
	// Note: ServeContent should be always last line, after all headers were set.
	http.ServeContent(w, r, filename, modtime, bytes.NewReader(*bs))
	return nil, nil
}
