package main

import (
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/license"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"net/http"
)

// LicenseList returns a list with all available licenses.
func LicenseList(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	return license.List(p, tx)
}
