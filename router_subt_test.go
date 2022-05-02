package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/gazebo-web/fuel-server/bundles/subt"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/migrate"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"gitlab.com/ignitionrobotics/web/ign-go/testhelpers"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Tests for subt related routes
func initilizeSubT(ctx context.Context) {
	if subt.BucketServerImpl == nil {
		subt.BucketServerImpl = &BucketServerMock{}
	}
	subt.Initialize(ctx, globals.Server.Db)
}

func setupSubT() {
	setupWithCustomInitalizer(initilizeSubT)
}

type BucketServerMock struct{}

// GetBucketName is an s3 implementation to get a bucket name in the cloud
func (s3b *BucketServerMock) GetBucketName(bucket string) string {
	fmt.Println("BucketServerMock GetBucketName invoked")
	return bucket
}

// Upload is an s3 implementation to upload files to a bucket
func (s3b *BucketServerMock) Upload(ctx context.Context, f io.Reader, bucket,
	fPath string) (*string, error) {
	fmt.Println("BucketServerMock Upload invoked")
	return sptr("url"), nil
}

// RemoveFile is an implementation to remove files from a bucket in S3
func (s3b *BucketServerMock) RemoveFile(ctx context.Context, bucket, fPath string) error {
	fmt.Println("BucketServerMock RemoveFile invoked")
	return nil
}

// GetPresignedURL returns presigned urls from S3 buckets.
func (s3b *BucketServerMock) GetPresignedURL(ctx context.Context, bucket,
	fPath string) (*string, error) {
	fmt.Println("BucketServerMock GetPresignedURL invoked")
	return sptr("a presigned url is here"), nil
}

// TestSubTRoutes tests several SubT routes
func TestSubTRoutes(t *testing.T) {
	setupSubT()
	// get the tests JWT
	jwtDef := newJWT(os.Getenv("IGN_TEST_JWT"))
	// create a random user using the default test JWT
	username := createSysAdminUser(t)
	defer removeUser(username, t)
	// create another user and set it as SubT Org admin
	subtadmJWT := createValidJWTForIdentity("subt-admin-1", t)
	subtadm := createUserWithJWT(subtadmJWT, t)
	defer removeUserWithJWT(subtadm, subtadmJWT, t)
	addUserToOrg(subtadm, "admin", subt.SubTPortalName, t)

	// now create 2 organizations that will act as SubT teams
	org := createOrganization(t)
	defer removeOrganization(org, t)
	org3 := createOrganizationWithName(t, "org name with spaces")
	defer removeOrganization(org3, t)

	// create a separate JWT but do not create a corresponding Fuel user for it.
	jwt2 := createValidJWTForIdentity("another-user-2", t)
	// create another user and make him admin of a team (org3)
	jwt3 := createValidJWTForIdentity("another-user-3", t)
	user3 := createUserWithJWT(jwt3, t)
	defer removeUserWithJWT(user3, jwt3, t)
	addUserToOrg(user3, "admin", org3, t)
	// create another user (a non competitor)
	jwt4 := createValidJWTForIdentity("non-competitor-user-4", t)
	user4 := createUserWithJWT(jwt4, t)
	defer removeUserWithJWT(user4, jwt4, t)
	// create another user (a 'member' of org)
	jwt5 := createValidJWTForIdentity("org-member-5", t)
	user5 := createUserWithJWT(jwt5, t)
	defer removeUserWithJWT(user5, jwt5, t)
	addUserToOrg(user5, "member", org, t)

	uri := "/1.0/subt/registrations"
	unauth := ign.NewErrorMessage(ign.ErrorUnauthorized)

	// Test submitting registrations
	submitRegistrationTestsData := []subtRegistrationTest{
		{uriTest{"no jwt", uri, nil, unauth, true}, ""},
		{uriTest{"invalid jwt token", uri, &testJWT{jwt: sptr("invalid")}, unauth, true},
			""},
		{uriTest{"no user in backend", uri, newJWT(jwt2),
			ign.NewErrorMessage(ign.ErrorAuthNoUser), false}, ""},
		{uriTest{"invalid organization name", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorNonExistentResource), false}, "noOrg"},
		{uriTest{"non org member cannot apply for the org", uri, newJWT(jwt4),
			unauth, false}, org},
		{uriTest{"only organization admins can apply for the org", uri, newJWT(jwt5),
			unauth, false}, org},
		{uriTest{"Cannot register the competition org as a participant", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, subt.SubTPortalName},
		// Note: the following test cases are inter-related, as the test for duplication.
		{uriTest{"Apply registration OK", uri, jwtDef, nil, false}, org},
		{uriTest{"Participant org already applied (pending)", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorResourceExists), false}, org},
		{uriTest{"User already submitted (pending) registrations for another org", uri,
			jwtDef, ign.NewErrorMessage(ign.ErrorResourceExists), false}, org3},
		{uriTest{"Another team registration OK", uri, newJWT(jwt3), nil, false}, org3},
	}
	for _, test := range submitRegistrationTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTRegistrationData(test, t)
		})
	}

	// Test the Get list of registrations (pending, done, etc)
	pendingURI := uri + "?status=pending"
	regListTests := []subtRegistrationListTest{
		{uriTest{"no jwt", pendingURI, nil, unauth, true}, nil},
		{uriTest{"invalid jwt token", pendingURI, &testJWT{jwt: sptr("invalid")},
			unauth, true}, nil},
		{uriTest{"no user in backend", pendingURI, newJWT(jwt2),
			ign.NewErrorMessage(ign.ErrorAuthNoUser), false}, nil},
		{uriTest{"test pagination support. Get page #1", uri + "?per_page=1&page=1", jwtDef, nil, false},
			[]string{org}},
		{uriTest{"invalid status in query", uri + "?status=invalid", jwtDef,
			ign.NewErrorMessage(ign.ErrorMissingField), false}, nil},
		{uriTest{"missing status in query should return pending ones", uri, jwtDef,
			nil, false}, []string{org, org3}},
		{uriTest{"user3 should only see registrations applied by him", uri, newJWT(jwt3),
			nil, false}, []string{org3}},
	}
	for _, test := range regListTests {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTRegistrationListData(test, t)
		})
	}

	// Now test processing the registrations (ie. accept or reject them)
	comp := subt.SubTPortalName
	resURI := fmt.Sprintf("%s/%s/", uri, comp)
	resolveRegistrations := []subtResolveRegistrationTest{
		{uriTest{"no jwt", resURI, nil, unauth, true}, org, nil},
		{uriTest{"invalid jwt token", resURI, &testJWT{jwt: sptr("invalid")}, unauth, true}, org, nil},
		{uriTest{"no body", resURI, jwtDef, ign.NewErrorMessage(ign.ErrorUnmarshalJSON),
			false}, org, nil},
		{uriTest{"unauthorized user cannot resolve registration", resURI, newJWT(jwt3), unauth,
			false}, org3, &subt.RegistrationUpdate{Resolution: subt.RegOpDone}},
		{uriTest{"Valid resolution to Done by sysadmin", resURI, jwtDef, nil,
			false}, org, &subt.RegistrationUpdate{Resolution: subt.RegOpDone}},
		{uriTest{"Cannot resolve a registration twice", resURI, jwtDef, ign.NewErrorMessage(ign.ErrorNameNotFound),
			false}, org, &subt.RegistrationUpdate{Resolution: subt.RegOpDone}},
		{uriTest{"Valid resolution to Rejected", resURI, newJWT(subtadmJWT), nil, false},
			org3, &subt.RegistrationUpdate{Resolution: subt.RegOpRejected}},
	}
	for _, test := range resolveRegistrations {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTResolveRegistrationData(test, t)
		})
	}

	// Test again getting the list of registrations
	doneURI := uri + "?status=done"
	rejectURI := uri + "?status=rejected"
	regListTests = []subtRegistrationListTest{
		{uriTest{"there should not be any pending registrations", uri, jwtDef,
			nil, false}, nil},
		{uriTest{"return registrations resolved with done", doneURI, jwtDef, nil,
			false}, []string{org}},
		{uriTest{"user3 should not see registrations not submitted by him", doneURI,
			newJWT(jwt3), nil, false}, nil},
		{uriTest{"return registrations resolved with reject", rejectURI, jwtDef, nil,
			false}, []string{org3}},
	}
	for _, test := range regListTests {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTRegistrationListData(test, t)
		})
	}

	// Now test again doing some new registrations
	org4 := createOrganization(t)
	defer removeOrganization(org4, t)
	jwtAdmOrg4 := createValidJWTForIdentity("admin-of-org4", t)
	admOrg4 := createUserWithJWT(jwtAdmOrg4, t)
	defer removeUserWithJWT(admOrg4, jwtAdmOrg4, t)
	addUserToOrg(admOrg4, "admin", org4, t)
	org5 := createOrganization(t)
	defer removeOrganization(org5, t)
	jwtAdmOrg5 := createValidJWTForIdentity("admin-of-org5", t)
	admOrg5 := createUserWithJWT(jwtAdmOrg5, t)
	defer removeUserWithJWT(admOrg5, jwtAdmOrg5, t)
	addUserToOrg(admOrg5, "admin", org5, t)
	submitRegistrationTestsData2 := []subtRegistrationTest{
		{uriTest{"rejected user can apply again", uri, newJWT(jwt3),
			nil, false}, org3},
		{uriTest{"User already has a DONE registration for another org", uri,
			jwtDef, ign.NewErrorMessage(ign.ErrorResourceExists), false}, org3},
		{uriTest{"org is already registered (Reg done)", uri, jwtDef,
			ign.NewErrorMessage(ign.ErrorResourceExists), false}, org},
		{uriTest{"Another team (org4) registration done by its admin", uri,
			newJWT(jwtAdmOrg4), nil, false}, org4},
		{uriTest{"Another team (org5) registration done by its admin", uri,
			newJWT(jwtAdmOrg5), nil, false}, org5},
	}
	for _, test := range submitRegistrationTestsData2 {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTRegistrationData(test, t)
		})
	}

	// Accept the new registration
	resolveRegistrations = []subtResolveRegistrationTest{
		{uriTest{"Valid resolution to Done by subt admin", resURI, newJWT(subtadmJWT),
			nil, false}, org3, &subt.RegistrationUpdate{Resolution: subt.RegOpDone}},
	}
	for _, test := range resolveRegistrations {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTResolveRegistrationData(test, t)
		})
	}

	// Now test DELETE pending registrations
	delOrg := resURI + org
	delOrg4 := resURI + org4
	delOrg5 := resURI + org5
	deleteRegistrationTestsData := []subtRegistrationDeleteTest{
		{uriTest{"already resolved registrations cannot be deleted", delOrg,
			newJWT(subtadmJWT), ign.NewErrorMessage(ign.ErrorNameNotFound), false}},
		{uriTest{"other user cannot delete a pending registration", delOrg4, newJWT(jwt3),
			unauth, false}},
		{uriTest{"same user can delete pending registration", delOrg4, jwtDef,
			nil, false}},
		{uriTest{"Subt admin can also delete a pending registration", delOrg5,
			newJWT(subtadmJWT), nil, false}},
	}
	for _, test := range deleteRegistrationTestsData {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTRegistrationDeleteData(test, t)
		})
	}

	// Test get list of participants
	pURI := "/1.0/subt/participants"
	participantsList := []subtParticipantsListTest{
		{uriTest{"no jwt", pURI, nil, unauth, true}, nil},
		{uriTest{"invalid jwt token", pURI, &testJWT{jwt: sptr("invalid")}, unauth, true}, nil},
		{uriTest{"no user in backend", pURI, newJWT(jwt2),
			ign.NewErrorMessage(ign.ErrorAuthNoUser), false}, nil},
		{uriTest{"test participants pagination support. Get page #1", pURI + "?per_page=1&page=1",
			jwtDef, nil, false}, []OrgData{{Name: org, Private: true}}},
		{uriTest{"non competition user should not see any participants", pURI,
			newJWT(jwt4), nil, false}, nil},
		{uriTest{"sysadmin should see all participants", pURI, jwtDef, nil, false},
			[]OrgData{{Name: org, Private: true}, {Name: org3, Private: true}}},
		{uriTest{"competition admin should see all participants", pURI, newJWT(subtadmJWT),
			nil, false}, []OrgData{{Name: org, Private: true}, {Name: org3, Private: true}}},
		{uriTest{"member of participant team should only see teams he belongs", pURI,
			newJWT(jwt3), nil, false}, []OrgData{{Name: org3, Private: true}}},
	}
	for _, test := range participantsList {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTParticipantsListData(test, t)
		})
	}

	//	Test deleting existing participants
	dpURI := "/1.0/subt/participants/subt/"
	participantDeletetions := []subtParticipantDeleteTest{
		{uriTest{"missing participant", dpURI, jwtDef, ign.NewErrorMessage(ign.ErrorNameNotFound), false}, org4},
		{uriTest{"valid deletion of participant", dpURI, jwtDef, nil, false}, org3},
	}
	for _, test := range participantDeletetions {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTParticipantDeleteData(test, t)
		})
	}

	// Resubmit some registrations for deleted participants
	resubmitRegistrationTestsData2 := []subtRegistrationTest{
		{uriTest{"Org3 registering again OK", uri, newJWT(jwt3), nil, false}, org3},
	}
	for _, test := range resubmitRegistrationTestsData2 {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTRegistrationData(test, t)
		})
	}

	// Now test processing the registrations again (ie. accept or reject them)
	// for those deleted participants that are registering again
	resolveRegistrationsAgain := []subtResolveRegistrationTest{
		{uriTest{"Valid resolution to Done by subt admin", resURI, newJWT(subtadmJWT),
			nil, false}, org3, &subt.RegistrationUpdate{Resolution: subt.RegOpDone}},
	}
	for _, test := range resolveRegistrationsAgain {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTResolveRegistrationData(test, t)
		})
	}

	// Test submitting log files
	lfURI := "/1.0/subt/logfiles"
	file := []igntest.FileDesc{{"log.txt", "test content"}}
	b := true
	logFileTests := []subtLogFileSubmitTest{
		{uriTest{"no jwt", lfURI, nil, unauth, true}, nil, nil},
		{uriTest{"no files", lfURI, jwtDef, ign.NewErrorMessage(ign.ErrorForm),
			false}, &subt.LogSubmission{Owner: org}, nil},
		{uriTest{"no logfile submission data", lfURI, jwtDef,
			ign.NewErrorMessage(ign.ErrorFormInvalidValue), false}, nil, file},
		{uriTest{"submit OK for org by member", lfURI, newJWT(jwt5), nil, false},
			&subt.LogSubmission{Owner: org}, file},
		{uriTest{"submit fails for non org member", lfURI, newJWT(jwt3), unauth, false},
			&subt.LogSubmission{Owner: org}, file},
		{uriTest{"submit fails for SubT admin", lfURI, newJWT(subtadmJWT), unauth, false},
			&subt.LogSubmission{Owner: org}, file},
		{uriTest{"submit OK for org3 by member", lfURI, newJWT(jwt3), nil, false},
			&subt.LogSubmission{Owner: org3, Description: "desc", Private: &b}, file},
		{uriTest{"another submit OK for org by member", lfURI, newJWT(jwt5), nil, false},
			&subt.LogSubmission{Owner: org}, file},
		{uriTest{"third submit OK for org by member", lfURI, newJWT(jwt5), nil, false},
			&subt.LogSubmission{Owner: org}, file},
	}
	for _, test := range logFileTests {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTLogFileSubmit(test, t)
		})
	}

	// Test scoring log files
	scoreLogFiles := []subtUpdateLogFileTest{
		{uriTest{"no jwt", lfURI, nil, unauth, true}, 1, nil},
		{uriTest{"invalid jwt token", lfURI, &testJWT{jwt: sptr("invalid")}, unauth,
			true}, 1, nil},
		{uriTest{"no body", lfURI, jwtDef, ign.NewErrorMessage(ign.ErrorUnmarshalJSON),
			false}, 1, nil},
		{uriTest{"unauthorized user cannot score logfile", lfURI, newJWT(jwt3), unauth,
			false}, 1, &subt.SubmissionUpdate{Status: subt.StDone, Score: 3}},
		{uriTest{"member of participant team cannot score own logfile", lfURI,
			newJWT(jwt5), unauth, false}, 1, &subt.SubmissionUpdate{Status: subt.StDone, Score: 3}},
		{uriTest{"updating a logfile allows setting Pending status again", lfURI,
			newJWT(subtadmJWT), nil,
			false}, 1, &subt.SubmissionUpdate{Status: subt.StForReview, Score: 3}},
		{uriTest{"Valid scoring by SubT admin (org)", lfURI, newJWT(subtadmJWT), nil,
			false}, 1, &subt.SubmissionUpdate{Status: subt.StDone, Score: 2.213}},
		{uriTest{"Valid scoring by system admin (org)", lfURI, jwtDef, nil,
			false}, 1, &subt.SubmissionUpdate{Status: subt.StRejected, Score: 35}},
		{uriTest{"Valid scoring by system admin for logfile 2 (org3)", lfURI, jwtDef, nil,
			false}, 2, &subt.SubmissionUpdate{Status: subt.StDone, Score: 30}},
		{uriTest{"Valid scoring by system admin for logfile 3 (org)", lfURI, jwtDef, nil,
			false}, 3, &subt.SubmissionUpdate{Status: subt.StDone, Score: 40}},
	}
	for _, test := range scoreLogFiles {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTUpdateLogFile(test, t)
		})
	}

	// Test getting a sigle logfile and downloading it
	expURL := "a presigned url is here"
	downloadLogFiles := []subtSingleLogFileTest{
		{uriTest{"no jwt", lfURI, nil, unauth, true}, 1, true, ""},
		{uriTest{"invalid jwt token", lfURI, &testJWT{jwt: sptr("invalid")}, unauth,
			true}, 1, true, ""},
		{uriTest{"member of another team cannot get logfile link", lfURI, newJWT(jwt3),
			unauth, false}, 1, true, ""},
		{uriTest{"member of participant team can get download link", lfURI, newJWT(jwt5),
			nil, false}, 1, true, expURL},
		{uriTest{"member of participant team can download logfile", lfURI, newJWT(jwt5),
			nil, false}, 1, false, expURL},
		{uriTest{"system admin can get logfile link", lfURI, jwtDef,
			nil, false}, 1, true, expURL},
	}
	for _, test := range downloadLogFiles {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTSingleLogFileTest(test, t)
		})
	}

	// Test the Get list of logfiles (InReview, Done, rejected)
	lfPending := lfURI + "?status=pending"
	participantLogs := func(p string) string {
		return fmt.Sprintf("/1.0/subt/participants/%s/logfiles", p)
	}
	logfileListTests := []subtLogFileListTest{
		{uriTest{"no jwt", lfPending, nil, unauth, true}, nil},
		{uriTest{"invalid jwt token", lfPending, &testJWT{jwt: sptr("invalid")},
			unauth, true}, nil},
		{uriTest{"no user in backend", lfPending, newJWT(jwt2),
			ign.NewErrorMessage(ign.ErrorAuthNoUser), false}, nil},
		{uriTest{"test pagination support. Get page #1", lfURI + "?per_page=1&page=1",
			newJWT(subtadmJWT), nil, false}, []uint{4}},
		{uriTest{"invalid status in query", lfURI + "?status=invalid", jwtDef,
			ign.NewErrorMessage(ign.ErrorMissingField), false}, nil},
		{uriTest{"missing status in query should return pending ones", lfURI, jwtDef,
			nil, false}, []uint{4}},
		{uriTest{"user5 should only see logfiles submitted by his team",
			lfURI + "?status=rejected", newJWT(jwt5), nil, false}, []uint{1}},
		{uriTest{"user3 should not see logfiles submitted by other teams",
			lfURI + "?status=rejected", newJWT(jwt3), nil, false}, []uint{}},
		{uriTest{"subt admin can see logfiles submitted by a participant",
			participantLogs(org) + "?status=rejected", newJWT(subtadmJWT), nil,
			false}, []uint{1}},
	}
	for _, test := range logfileListTests {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTLogFileListData(test, t)
		})
	}

	// Test Deleting Log files
	deleteLogFileTests := []subtLogFileDeleteTest{
		{uriTest{"no jwt", lfURI, nil, unauth, true}, 1},
		{uriTest{"invalid jwt token", lfURI, &testJWT{jwt: sptr("invalid")}, unauth,
			true}, 1},
		{uriTest{"other user cannot delete logfile", lfURI, newJWT(jwt3), unauth,
			false}, 1},
		{uriTest{"creator user cannot delete logfile", lfURI, newJWT(jwt5), unauth,
			false}, 1},
		{uriTest{"Competition admins cannot delete logfile either", lfURI,
			newJWT(subtadmJWT), unauth, false}, 1},
		{uriTest{"Only system admin can delete logfiles", lfURI,
			jwtDef, nil, false}, 1},
	}
	for _, test := range deleteLogFileTests {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithSubTLogFileDelete(test, t)
		})
	}

	// Create CompetitionScore entries from log scores
	migrate.LogFileScoresToCompetitionScore(globals.Server.Db, "Tunnel Qualifiers")
	// Test the Get the leaderboard (it should be publicly accessible)
	lbURI := "/1.0/subt/leaderboard"
	leaderboardTests := []leaderboardTest{
		{uriTest{"leaderboard - no jwt - should be OK", lbURI, nil, nil, false},
			[]*string{&org, &org3}, []float32{40.0, 30.0}},
		{uriTest{"leaderboard - invalid jwt token", lbURI, &testJWT{jwt: sptr("invalid")},
			unauth, true}, nil, nil},
		{uriTest{"leaderboard - no user in backend", lbURI, newJWT(jwt2), nil, false},
			[]*string{&org, &org3}, []float32{40.0, 30.0}},
		{uriTest{"get leaderboard OK by system admin", lbURI, jwtDef, nil, false},
			[]*string{&org, &org3}, []float32{40.0, 30.0}},
		{uriTest{"get leaderboard OK by admin", lbURI, newJWT(subtadmJWT), nil, false},
			[]*string{&org, &org3}, []float32{40.0, 30.0}},
		{uriTest{"get leaderboard OK by participant", lbURI, newJWT(jwt3), nil, false},
			[]*string{&org, &org3}, []float32{40.0, 30.0}},
		{uriTest{"get leaderboard by NON participant", lbURI, newJWT(jwt4), nil, false},
			[]*string{&org, &org3}, []float32{40.0, 30.0}},
	}
	for _, test := range leaderboardTests {
		t.Run(test.testDesc, func(t *testing.T) {
			runSubTestWithLeaderboardTestData(test, t)
		})
	}

	// Test leaderboard scores with an organization filter
	globals.LeaderboardOrganizationFilter = []string{org3}
	for i, filter := range globals.LeaderboardOrganizationFilter {
		globals.LeaderboardOrganizationFilter[i] = strings.ToLower(filter)
	}
	test := leaderboardTest{
		uriTest{"leaderboard - env var organization filter", lbURI, nil, nil, false}, []*string{&org}, []float32{40.0},
	}
	t.Run(test.testDesc, func(t *testing.T) {
		runSubTestWithLeaderboardTestData(test, t)
	})
	globals.LeaderboardOrganizationFilter = []string{}

	// Test leaderboard scores with a circuit filter
	globals.LeaderboardCircuitFilter = []string{"Tunnel Qualifiers"}
	for i, filter := range globals.LeaderboardOrganizationFilter {
		globals.LeaderboardOrganizationFilter[i] = strings.ToLower(filter)
	}
	test = leaderboardTest{
		uriTest{"leaderboard - env var circuit filter", lbURI, nil, nil, false}, []*string{}, []float32{},
	}
	t.Run(test.testDesc, func(t *testing.T) {
		runSubTestWithLeaderboardTestData(test, t)
	})
	globals.LeaderboardCircuitFilter = []string{}

}

// subtRegistrationTest includes the input and expected output for a
// TestSubTRegistration test case.
type subtRegistrationTest struct {
	uriTest
	organization string
}

// runSubTestWithSubTRegistrationData tries to apply a registration for SubT
func runSubTestWithSubTRegistrationData(test subtRegistrationTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	b := new(bytes.Buffer)
	rc := subt.RegistrationCreate{Participant: test.organization}
	json.NewEncoder(b).Encode(rc)

	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "POST", Route: test.URL, Body: b, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var reg subt.Registration
		require.NoError(t, json.Unmarshal(*bslice, &reg), "Unable to unmarshal response", string(*bslice))
		assert.Nil(t, reg.ResolvedAt)
		require.NotNil(t, reg.Participant)
		assert.Equal(t, test.organization, *reg.Participant)
		assert.Equal(t, subt.SubTPortalName, *reg.Competition)
		assert.Equal(t, subt.RegOpPending, subt.RegStatus(*reg.Status))
	}
}

// subtResolveRegistrationTest is used to test the resolve part of a registration.
type subtResolveRegistrationTest struct {
	uriTest
	organization string
	ru           *subt.RegistrationUpdate
}

// runSubTestWithSubTResolveRegistrationData tries to resolve a registration
func runSubTestWithSubTResolveRegistrationData(test subtResolveRegistrationTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	var b *bytes.Buffer
	if test.ru != nil {
		b = new(bytes.Buffer)
		json.NewEncoder(b).Encode(*test.ru)
	}
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	uri := test.URL + test.organization
	igntest.AssertRoute("OPTIONS", uri, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "PATCH", Route: uri, Body: b, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var reg subt.Registration
		require.NoError(t, json.Unmarshal(*bslice, &reg), "Unable to unmarshal response", string(*bslice))
		assert.NotNil(t, reg.ResolvedAt)
		assert.Equal(t, test.organization, *reg.Participant)
		assert.Equal(t, subt.SubTPortalName, *reg.Competition)
		assert.Equal(t, test.ru.Resolution, subt.RegStatus(*reg.Status))
	}
}

// subtRegistrationDeleteTest is used to test the deletion of a pending registration
type subtRegistrationDeleteTest struct {
	uriTest
}

// runSubTestWithSubTRegistrationDeleteData tries to delete a registration
func runSubTestWithSubTRegistrationDeleteData(test subtRegistrationDeleteTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "DELETE", Route: test.URL, Body: nil, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var reg subt.Registration
		require.NoError(t, json.Unmarshal(*bslice, &reg), "Unable to unmarshal response", string(*bslice))
		assert.NotEmpty(t, reg.CreatedAt)
	}
}

// subtRegistrationListTest is used to test the get list of registrations.
type subtRegistrationListTest struct {
	uriTest
	// the organizations (participants) associated to the registrations
	organizations []string
}

func runSubTestWithSubTRegistrationListData(test subtRegistrationListTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var regs subt.Registrations
		require.NoError(t, json.Unmarshal(*bslice, &regs), "Unable to unmarshal response", string(*bslice))
		require.Len(t, regs, len(test.organizations))
		for i, o := range test.organizations {
			assert.Equal(t, o, *regs[i].Participant)
		}
	}
}

// subtParticipantsListTest is used to test the get list of participants.
type subtParticipantsListTest struct {
	uriTest
	organizations []OrgData
}

type OrgData struct {
	Name    string
	Private bool
}

func runSubTestWithSubTParticipantsListData(test subtParticipantsListTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var ps users.OrganizationResponses
		require.NoError(t, json.Unmarshal(*bslice, &ps), "Unable to unmarshal response", string(*bslice))
		require.Len(t, ps, len(test.organizations))
		for i, o := range test.organizations {
			assert.Equal(t, o.Name, ps[i].Name, "with id %d", i)
			assert.Equal(t, o.Private, ps[i].Private, "Different in expected VS got Private data", i)
		}
	}
}

type subtParticipantDeleteTest struct {
	uriTest
	Name string
}

func runSubTestWithSubTParticipantDeleteData(test subtParticipantDeleteTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL+test.Name, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "DELETE", Route: test.URL + test.Name, Body: nil, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		participants := subt.CompetitionParticipants{}
		q := globals.Server.Db.Table("competition_participants").Select("competition_participants.*").Where("competition_participants.owner = ?", test.Name)
		q.Find(&participants)
		assert.Empty(t, participants, "Participant should have been deleted")
	}
}

// subtLogFileSubmitTest tests sumbitting log files
type subtLogFileSubmitTest struct {
	uriTest
	ls    *subt.LogSubmission
	files []igntest.FileDesc
}

func runSubTestWithSubTLogFileSubmit(test subtLogFileSubmitTest, t *testing.T) {
	var params map[string]string
	if test.ls != nil {
		params = map[string]string{
			"owner":       test.ls.Owner,
			"description": test.ls.Description,
		}
		if test.ls.Private != nil {
			params["private"] = strconv.FormatBool(*test.ls.Private)
		}
	}
	jwt := getJWTToken(t, test.jwtGen)
	expEm, _ := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	code, bslice := postWithArgs(t, test.URL, jwt, params, test.files)
	require.Equal(t, expStatus, code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var lf subt.LogFile
		require.NoError(t, json.Unmarshal(*bslice, &lf), "Unable to unmarshal response", string(*bslice))
		require.Equal(t, test.ls.Owner, *lf.Owner)
		require.NotEmpty(t, lf.ID)
		assert.NotNil(t, lf.Location)
		assert.True(t, *lf.Private)
		assert.Equal(t, subt.SubTPortalName, *lf.Competition)
		assert.Equal(t, (*float32)(nil), lf.Score)
		assert.Equal(t, subt.StForReview, subt.SubmissionStatus(*lf.Status))
	}
}

// subtUpdateLogFileTest is used to test the scoring part of a log file.
type subtUpdateLogFileTest struct {
	uriTest
	logID uint
	su    *subt.SubmissionUpdate
}

func runSubTestWithSubTUpdateLogFile(test subtUpdateLogFileTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	var b *bytes.Buffer
	if test.su != nil {
		b = new(bytes.Buffer)
		json.NewEncoder(b).Encode(*test.su)
	}
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	uri := fmt.Sprintf("%s/%d", test.URL, test.logID)
	igntest.AssertRoute("OPTIONS", uri, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "PATCH", Route: uri, Body: b, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var log subt.LogFile
		require.NoError(t, json.Unmarshal(*bslice, &log), "Unable to unmarshal response", string(*bslice))
		assert.NotNil(t, log.ResolvedAt)
		require.Equal(t, test.logID, log.ID)
		assert.True(t, *log.Private)
		assert.Equal(t, subt.SubTPortalName, *log.Competition)
		assert.Equal(t, test.su.Status, subt.SubmissionStatus(*log.Status))
		assert.Equal(t, test.su.Score, *log.Score)
	}
}

// subtLogFileDeleteTest is used to test the deletion of a log file
type subtLogFileDeleteTest struct {
	uriTest
	logID uint
}

func runSubTestWithSubTLogFileDelete(test subtLogFileDeleteTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	uri := fmt.Sprintf("%s/%d", test.URL, test.logID)
	igntest.AssertRoute("OPTIONS", uri, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "DELETE", Route: uri, Body: nil, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var log subt.LogFile
		require.NoError(t, json.Unmarshal(*bslice, &log), "Unable to unmarshal response", string(*bslice))
		assert.Equal(t, test.logID, log.ID)
		assert.Equal(t, subt.SubTPortalName, *log.Competition)
	}
}

// subtLogFileListTest is used to test the get list of log files.
type subtLogFileListTest struct {
	uriTest
	ids []uint
}

func runSubTestWithSubTLogFileListData(test subtLogFileListTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var logs subt.LogFiles
		require.NoError(t, json.Unmarshal(*bslice, &logs), "Unable to unmarshal response", string(*bslice))
		require.Len(t, logs, len(test.ids))
		for i, id := range test.ids {
			assert.Equal(t, id, logs[i].ID)
			assert.True(t, *logs[i].Private)
		}
	}
}

// subtSingleLogFileTest is used to test getting a single log file
type subtSingleLogFileTest struct {
	uriTest
	logID    uint
	linkOnly bool
	expURL   string
}

func runSubTestWithSubTSingleLogFileTest(test subtSingleLogFileTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	uri := fmt.Sprintf("%s/%d/file", test.URL, test.logID)
	if test.linkOnly {
		uri += "?link=true"
	} else if expStatus == http.StatusOK {
		expStatus = http.StatusTemporaryRedirect
	}
	igntest.AssertRoute("OPTIONS", uri, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "GET", Route: uri, Body: nil, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && expStatus != http.StatusTemporaryRedirect {
		if !test.ignoreErrorBody {
			igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
		}
	} else {
		if test.linkOnly {
			// we assume expected http code is StatusOK
			var u string
			require.NoError(t, json.Unmarshal(*bslice, &u), "Unable to unmarshal response", string(*bslice))
			assert.NotEmpty(t, u)
			assert.Equal(t, test.expURL, u)
		} else {
			// Redirect expected
			assert.Equal(t, http.StatusTemporaryRedirect, resp.RespRecorder.Code)
		}
	}
}

// leaderboardTest is used to test the get list of log files.
type leaderboardTest struct {
	uriTest
	owners []*string
	scores []float32
}

func runSubTestWithLeaderboardTestData(test leaderboardTest, t *testing.T) {
	jwt := getJWTToken(t, test.jwtGen)
	expEm, expCt := errMsgAndContentType(test.expErrMsg, ctJSON)
	expStatus := expEm.StatusCode
	igntest.AssertRoute("OPTIONS", test.URL, http.StatusOK, t)
	reqArgs := igntest.RequestArgs{Method: "GET", Route: test.URL, Body: nil, SignedToken: jwt}
	resp := igntest.AssertRouteMultipleArgsStruct(reqArgs, expStatus, expCt, t)
	bslice := resp.BodyAsBytes
	require.Equal(t, expStatus, resp.RespRecorder.Code)
	if expStatus != http.StatusOK && !test.ignoreErrorBody {
		igntest.AssertBackendErrorCode(t.Name(), bslice, expEm.ErrCode, t)
	} else if expStatus == http.StatusOK {
		var lb []subt.LeaderboardParticipant
		require.NoError(t, json.Unmarshal(*bslice, &lb), "Unable to unmarshal response", string(*bslice))
		require.Len(t, lb, len(test.owners))
		for i, o := range test.owners {
			assert.Equal(t, o, lb[i].Owner)
			assert.Equal(t, test.scores[i], *lb[i].Score)
		}
	}
}
