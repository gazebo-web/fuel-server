package main

import (
	"bitbucket.org/ignitionrobotics/ign-fuelserver/bundles/license"
	"bitbucket.org/ignitionrobotics/ign-go"
	"bitbucket.org/ignitionrobotics/ign-go/testhelpers"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"net/http"
	"os"
	"testing"
)

// Tests for License related routes

// licenseListTest defines a TestGetLicenses test case.
type licenseListTest struct {
	uriTest
	// expected license count in response
	expCount int
	// expected names of returned licenses
	expNames []string
}

func TestGetLicenses(t *testing.T) {
	// General test setup
	setup()
	myJWT := os.Getenv("IGN_TEST_JWT")
	defaultJWT := newJWT(myJWT)

	uri := "/1.0/licenses"

	expNames := []string{
		"Creative Commons - Public Domain",
		"Creative Commons - Attribution",
		"Creative Commons - Attribution - Share Alike",
		"Creative Commons - Attribution - No Derivatives",
		"Creative Commons - Attribution - Non Commercial",
		"Creative Commons - Attribution - Non Commercial - Share Alike",
		"Creative Commons - Attribution - Non Commercial - No Derivatives",
	}
	expNamesPage2 := []string{
		"Creative Commons - Attribution - Share Alike",
		"Creative Commons - Attribution - No Derivatives",
	}

	licenseListTestsData := []licenseListTest{
		{uriTest{"all licenses", uri, nil, nil, false}, 7, expNames},
		// WITH PAGINATION
		{uriTest{"get page #1", uri + "?per_page=1&page=1", nil, nil, false}, 1, []string{"Creative Commons - Public Domain"}},
		{uriTest{"get page #2 size 2", uri + "?per_page=2&page=2", nil, nil, false}, 2, expNamesPage2},
		{uriTest{"invalid page", uri + "?per_page=1&page=8", nil, ign.NewErrorMessage(ign.ErrorPaginationPageNotFound), false}, 0, nil},
	}

	for _, test := range licenseListTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubtestWithLicenseListTestData(t, test)
		})
		// Now run the same test case but adding a JWT, if needed
		if test.jwtGen == nil {
			test.jwtGen = defaultJWT
			test.testDesc += "[with JWT]"
			t.Run(test.testDesc, func(t *testing.T) {
				runSubtestWithLicenseListTestData(t, test)
			})
		}
	}
}

func runSubtestWithLicenseListTestData(t *testing.T, test licenseListTest) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	assert.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var lics license.Licenses
		assert.NoError(t, json.Unmarshal(*bslice, &lics), "Unable to get all licenses: %s", string(*bslice))
		assert.Len(t, lics, test.expCount, "There should be %d licenses. Got: %d", test.expCount, len(lics))
		if test.expCount > 0 {
			// check root node paths
			for i, l := range lics {
				assert.Equal(t, test.expNames[i], *l.Name, "License (index %d) name should be [%s] but got [%s]", i, test.expNames[i], *l.Name)
			}
		}
	}
}
