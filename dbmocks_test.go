package main

import (
	"database/sql"
	"database/sql/driver"
	mocket "github.com/Selvatico/go-mocket"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
)

const (
	DriverFailAtBegin = "FAIL_AT_BEGIN_FAKE_DRIVER"
)

// SetGlobalDB is a helper function to change the global DB used
// by the server.
func SetGlobalDB(db *gorm.DB) {
	globals.Server.Db = db
}

// SetupDbMockCatcher registers custom DB drivers that support mocks
func SetupDbMockCatcher() *gorm.DB {
	// Register fake driver
	mocket.Catcher.Register()
	// Also register our custom fail at begin driver
	RegisterFailAtBeginDriver()

	mocket.Catcher.Logging = false
	mockDb, _ := gorm.Open(mocket.DRIVER_NAME, "any_string")

	return mockDb
}

// SetupCommonMockResponses initializes some mock responses to common queries.
func SetupCommonMockResponses(testUser string) {
	// Set up some common DB responses
	commonUniqueOwnerReply := []map[string]interface{}{{"id": "10", "name": testUser}}
	commonUserReply := []map[string]interface{}{{"id": "101", "username": testUser, "identity": "test-user-identity"}}
	commonModelReply := []map[string]interface{}{{"id": "100", "uuid": "uuid-string", "name": "model-name", "private": false}}
	commonModelLikeReply := []map[string]interface{}{{"id": "2", "user_id": "101", "model_id": "100"}}

	mocket.Catcher.Reset()
	mocket.Catcher.Attach([]*mocket.FakeResponse{
		{
			Pattern:  "SELECT * FROM \"unique_owners\"  WHERE",
			Response: commonUniqueOwnerReply,
			Once:     false,
		},
		{
			Pattern:  "SELECT * FROM \"users\"  WHERE",
			Response: commonUserReply,
			Once:     false,
		},
		{
			Pattern:  "SELECT * FROM \"models\"  WHERE",
			Response: commonModelReply,
			Once:     false,
		},
		{
			Pattern:  "SELECT FROM \"model_likes\"  WHERE",
			Response: commonModelLikeReply,
			Once:     false,
		},
	})
}

// SetupMockCountModelLikes adds a new mock query that returns 1 when counting model likes.
func SetupMockCountModelLikes() {
	mocket.Catcher.NewMock().WithQuery("SELECT count(*) FROM \"model_likes\"  WHERE").WithRowsNum(1).WithReply([]map[string]interface{}{{"count": "1"}})
}

// SetupMockBadCommit configures the DB mock fail on transaction Commit().
func SetupMockBadCommit() {
	mocket.HookBadCommit = func() bool { return true }
}

// ClearMockBadCommit removes the bad commit hook
func ClearMockBadCommit() {
	mocket.HookBadCommit = nil
}

// --------------------------------------------
// --------------------------------------------
// --------------------------------------------

// NewFailAtBeginConn opens and returns a FailAtBegin fake database to be
// used with GORM.
func NewFailAtBeginConn() *gorm.DB {
	c, _ := gorm.Open(DriverFailAtBegin, "any_string")
	return c
}

// FailAtBeginFakeDriver is a fake driver to create DB connections
// that will fail when Begin() is called on them
type FailAtBeginFakeDriver struct {
}

// RegisterFailAtBeginDriver registers a custom driver to be used by sql package.
func RegisterFailAtBeginDriver() {
	driversList := sql.Drivers()
	for _, name := range driversList {
		if name == DriverFailAtBegin {
			return
		}
	}
	sql.Register(DriverFailAtBegin, FailAtBeginFakeDriver{})
}

// FailAtBeginFakeConn is a fake connection that fails on each call to Begin().
type FailAtBeginFakeConn struct {
	*mocket.FakeConn
}

// Open returns a new (fake) connection to the database.
func (d FailAtBeginFakeDriver) Open(database string) (driver.Conn, error) {
	return &FailAtBeginFakeConn{}, nil
}

// Begin func just returns an driver.ErrBadConn error.
func (c *FailAtBeginFakeConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrBadConn
}

// Close func just returns success.
func (c *FailAtBeginFakeConn) Close() (err error) {
	return nil
}
