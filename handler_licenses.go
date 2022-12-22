package main

import (
	"github.com/gazebo-web/fuel-server/bundles/license"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
	"net/http"
)

// LicenseList returns a list with all available licenses.
func LicenseList(p *gz.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg) {

	return license.List(p, tx)
}
