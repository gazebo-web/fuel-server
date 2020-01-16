package main

import (
	"bitbucket.org/ignitionrobotics/ign-fuelserver/bundles/license"
	"bitbucket.org/ignitionrobotics/ign-fuelserver/bundles/users"
	"bitbucket.org/ignitionrobotics/ign-go"
	"github.com/jinzhu/gorm"
	"net/http"
)

// LicenseList returns a list with all available licenses.
func LicenseList(p *ign.PaginationRequest, user *users.User, tx *gorm.DB,
	w http.ResponseWriter, r *http.Request) (interface{}, *ign.PaginationResult, *ign.ErrMsg) {

	return license.List(p, tx)
}
