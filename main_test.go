package main

import (
	"bitbucket.org/ignitionrobotics/ign-fuelserver/globals"
	"bitbucket.org/ignitionrobotics/ign-go"
	"context"
	"log"
	"os"
	"testing"
)

// This function applies to ALL tests in the application.
// It will run the test and then clean the database.
func TestMain(m *testing.M) {
	code := m.Run()
	packageTearDown(nil)
	log.Println("Cleaned database tables after all tests")
	os.Exit(code)
}

// Clean up our mess
func packageTearDown(ctx context.Context) {
	if ctx == nil {
		ctx = ign.NewContextWithLogger(context.Background(), ign.NewLoggerNoRollbar("test", ign.VerbosityDebug))
	}
	cleanDBTables(ctx)
	// Remove all created folders
	os.RemoveAll(globals.ResourceDir)
}

func cleanDBTables(ctx context.Context) {
	DBDropModels(ctx, globals.Server.Db)
	DBMigrate(ctx, globals.Server.Db)
	// After removing tables we can ask casbin to re initialize
	if err := globals.Permissions.Reload(sysAdminForTest); err != nil {
		log.Fatal("Error reloading casbin policies", err)
	}
	// Apply custom indexes. Eg: fulltext indexes
	DBAddCustomIndexes(ctx, globals.Server.Db)
}
