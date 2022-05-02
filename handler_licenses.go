package main

import (
	"github.com/jinzhu/gorm"
	"github.com/gazebo-web/fuel-server/bundles/license"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"net/http"
)

// LicenseList returns a list with all available licenses.
func LicenseList(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	return license.List(p, tx)
}
