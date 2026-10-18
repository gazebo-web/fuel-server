package main

import (
	"context"
	"github.com/gazebo-web/fuel-server/cmd/token-generator/generator"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/golang-jwt/jwt/v5"

	"log"
	"os"
	"testing"
	"time"
)

// This function applies to ALL tests in the application.
// It will run the test and then clean the database.
func TestMain(m *testing.M) {
	resignTestJWT()
	code := m.Run()
	packageTearDown(context.TODO())
	log.Println("Cleaned database tables after all tests")
	os.Exit(code)
}

// Clean up our mess
func packageTearDown(ctx context.Context) {
	if ctx == nil {
		ctx = gz.NewContextWithLogger(context.Background(), gz.NewLoggerNoRollbar("test", gz.VerbosityDebug))
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

// resignTestJWT gives IGN_TEST_JWT (also read by gz-go's test helpers) an
// expiry, which package auth requires: the token is re-signed with the test
// key, keeping its other claims.
func resignTestJWT() {
	tok := os.Getenv("IGN_TEST_JWT")
	if tok == "" {
		return
	}
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(tok, claims); err != nil {
		log.Fatal("IGN_TEST_JWT: ", err)
	}
	if _, ok := claims["exp"]; ok {
		return
	}
	claims["exp"] = time.Now().Add(24 * time.Hour).Unix()
	key := []byte("-----BEGIN RSA PRIVATE KEY-----\n" + os.Getenv("TOKEN_GENERATOR_PRIVATE_RSA256_KEY") +
		"\n-----END RSA PRIVATE KEY-----")
	signed, err := generator.GenerateTokenRSA256(key, claims)
	if err != nil {
		log.Fatal("re-sign IGN_TEST_JWT: ", err)
	}
	os.Setenv("IGN_TEST_JWT", signed)
}
