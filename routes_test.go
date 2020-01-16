package main

import (
	"bitbucket.org/ignitionrobotics/ign-fuelserver/globals"
	"bitbucket.org/ignitionrobotics/ign-go"
	"bitbucket.org/ignitionrobotics/ign-go/testhelpers"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

// Generic Routes tests

var routesAPI2 = ign.Routes{
	// Test api 2.0
	ign.Route{
		"Test api 2.0",
		"example route",
		"/testapi",
		ign.AuthHeadersOptional,
		ign.Methods{
			ign.Method{
				"GET",
				"Test api",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(handlerAPI2)},
				},
			},
		},
		ign.SecureMethods{},
	},
}

func handlerAPI2(tx *gorm.DB, w http.ResponseWriter, r *http.Request) *ign.ErrMsg {
	fmt.Println("TestApi2")
	return nil
}

// Just invokes an invalid route
func TestInvalidRoute(t *testing.T) {
	// General test setup
	setup()
	igntest.InvalidRouteTestHelper(t)
}

// Test the autogenerated OPTIONS urls
func TestOptions(t *testing.T) {
	// General test setup
	setup()

	var names []string
	var uris []string
	for _, r := range routes {
		names = append(names, r.Name)
		prefixedURI := "/1.0" + r.URI
		uris = append(uris, prefixedURI)
	}
	igntest.OptionsTestHelper(uris, names, t)
}

// Just invokes an invalid route
func TestSupportForMultipleAPIVersions(t *testing.T) {
	origRouter := globals.Server.Router
	defer globals.Server.SetRouter(origRouter)

	// General test setup
	setup()
	v1prefix := "/1.0"
	v2prefix := "/2.0"
	s := globals.Server
	mainRouter := ign.NewRouter()
	// Example with 2 simultaneous APIs
	r := mainRouter.PathPrefix(v1prefix).Subrouter()
	s.ConfigureRouterWithRoutes(v1prefix, r, routes)
	// Now create a sub router , enabled with /2.0/
	sub := mainRouter.PathPrefix(v2prefix).Subrouter()
	s.ConfigureRouterWithRoutes(v2prefix, sub, routesAPI2)
	s.SetRouter(mainRouter)

	// Set the new test router
	igntest.SetupTest(mainRouter)

	// Test the OPTIONS routes with the 2 apis
	var names []string
	var uris []string
	for _, r := range routes {
		names = append(names, r.Name)
		prefixedURI := v1prefix + r.URI
		uris = append(uris, prefixedURI)
	}
	for _, r := range routesAPI2 {
		names = append(names, r.Name)
		prefixedURI := v2prefix + r.URI
		uris = append(uris, prefixedURI)
	}
	igntest.OptionsTestHelper(uris, names, t)
}

// Tests that the sqlTx error message has not changed.
// NOTE: We need this test
// because in the server code we compare against the error message to
// detect is the underlying error is a TX error from the sql driver.
func TestSqlTxError(t *testing.T) {
	tx := globals.Server.Db.Begin()
	assert.NoError(t, tx.Rollback().Error)
	// should fail on subsequent calls to rollback or commit
	assert.Error(t, tx.Rollback().Error)
	assert.True(t, ign.IsSQLTxError(tx.Error))
	assert.Error(t, tx.Commit().Error)
	assert.True(t, ign.IsSQLTxError(tx.Error))

	tx = globals.Server.Db.Begin()
	assert.NoError(t, tx.Commit().Error)
	// should fail on subsequent calls to rollback or commit
	assert.Error(t, tx.Rollback().Error)
	assert.True(t, ign.IsSQLTxError(tx.Error))
	assert.Error(t, tx.Commit().Error)
	assert.True(t, ign.IsSQLTxError(tx.Error))

	tx = globals.Server.Db.Begin()
	assert.Error(t, tx.Begin().Error)
}
