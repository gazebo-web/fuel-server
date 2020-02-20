// Package main Ignition Fuel Server RESET API
//
// This package provides a REST API to the Ignition Fuel server.
//
// Schemes: https
// Host: staging-api.ignitionfuel.org
// BasePath: /1.0
// Version: 0.1.0
// License: Apache 2.0
// Contact: info@openrobotics.org
//
// swagger:meta
// go:generate swagger generate spec
package main

// \todo Add in the following to the comments at the top of this file to enable
// security
//
// SecurityDefinitions:
//   token:
//     type: apiKey
//     name: authorization
//     in: header
//     description: Ignition Fuel token
//   auth0:
//     type: apiKey
//     name: authorization
//     in: header
//     description: Auth0 token. Note, It must start with 'Bearer '
//

// Import this file's dependencies
import (
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/subt"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/fuelserver/migrate"
	"gitlab.com/ignitionrobotics/web/fuelserver/permissions"
	"gitlab.com/ignitionrobotics/web/fuelserver/vcs"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"context"
	"flag"
	"github.com/go-playground/form"
	"gopkg.in/go-playground/validator.v9"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
)

// Impl note: we move this as a constant as it is used by tests.
const sysAdminForTest = "rootfortests"

/////////////////////////////////////////////////
/// Initialize this package
///
/// Environment variables:
///    IGN_DB_USERNAME  : Mysql username
///    IGN_DB_PASSWORD  : Mysql password
///    IGN_DB_ADDRESS   : Mysql address (host:port)
///    IGN_DB_NAME      : Mysql database name (such as "fuel")
///    IGN_FUEL_RESOURCE_DIR : Directory with all resources (models, worlds)
///    AUTH0_RSA256_PUBLIC_KEY   : Auth0 public RSA 256 key
func init() {
	var err error
	var popPath string
	var isGoTest bool
	var auth0RsaPublickey string

	verbosity := ign.VerbosityWarning
	if verbStr, verr := ign.ReadEnvVar("IGN_FUEL_VERBOSITY"); verr == nil {
		verbosity, _ = strconv.Atoi(verbStr)
	}

	logStd := ign.ReadStdLogEnvVar()
	logger := ign.NewLogger("init", logStd, verbosity)
	logCtx := ign.NewContextWithLogger(context.Background(), logger)

	isGoTest = flag.Lookup("test.v") != nil

	// Get the root resource directory.
	if globals.ResourceDir, err = ign.ReadEnvVar("IGN_FUEL_RESOURCE_DIR"); err != nil {
		log.Fatal("Missing IGN_FUEL_RESOURCE_DIR env variable. Resources will not be available. Quitting.")
	}

	if isGoTest {
		// Override globals.ResourceDir with a newly created /tmp folder
		globals.ResourceDir, err = ioutil.TempDir("", "fuel-")
		if err != nil {
			log.Fatal("Could not initialize test globals.ResourceDir. Resources will not be available")
		}
	}

	// Get the auth0 credentials.
	if auth0RsaPublickey, err = ign.ReadEnvVar("AUTH0_RSA256_PUBLIC_KEY"); err != nil {
		logger.Info("Missing AUTH0_RSA256_PUBLIC_KEY env variable. Authentication will not work.")
	}

	globals.Server, err = ign.Init(auth0RsaPublickey, "")
	// Create the main Router and set it to the server.
	// Note: here it is the place to define multiple APIs
	s := globals.Server
	mainRouter := ign.NewRouter()
	apiPrefix := "/" + globals.APIVersion
	r := mainRouter.PathPrefix(apiPrefix).Subrouter()
	s.ConfigureRouterWithRoutes(apiPrefix, r, routes)

	// Now create a sub router for SubT, enabled with /subt/
	subtPrefix := apiPrefix + "/subt"
	sub := mainRouter.PathPrefix(subtPrefix).Subrouter()
	s.ConfigureRouterWithRoutes(subtPrefix, sub, subTRoutes)

	globals.Server.SetRouter(mainRouter)

	globals.FlagsEmailRecipient, _ = ign.ReadEnvVar("IGN_FLAGS_EMAIL_TO")
	globals.FlagsEmailSender, _ = ign.ReadEnvVar("IGN_FLAGS_EMAIL_FROM")
	globals.Validate = initValidator()
	globals.FormDecoder = form.NewDecoder()

	// Initialize leaderboard filters
	globals.LeaderboardOrganizationFilter = strings.Split(os.Getenv("IGN_FUEL_TEST_ORGANIZATIONS"), ",")
	for i, filter := range globals.LeaderboardOrganizationFilter {
		globals.LeaderboardOrganizationFilter[i] = strings.ToLower(filter)
	}
	globals.LeaderboardCircuitFilter = strings.Split(os.Getenv("IGN_FUEL_HIDE_CIRCUIT_SCORES"), ",")
	for i, filter := range globals.LeaderboardCircuitFilter {
		globals.LeaderboardCircuitFilter[i] = strings.ToLower(filter)
	}

	// Use go-git for our VCS.
	globals.VCSRepoFactory = func(ctx context.Context, dirpath string) vcs.VCS {
		return vcs.GoGitVCS{}.NewRepo(dirpath)
	}

	// initialize permissions
	// override sys admin for tests
	var sysAdmin string
	if isGoTest {
		sysAdmin = sysAdminForTest
	} else {
		sysAdmin, _ = ign.ReadEnvVar("IGN_FUEL_SYSTEM_ADMIN")
	}
	if sysAdmin == "" {
		logger.Info("No IGN_FUEL_SYSTEM_ADMIN enivironment variable set. " +
			"No system administrator role will be created")
	}
	globals.Permissions = &permissions.Permissions{}
	globals.Permissions.Init(globals.Server.Db, sysAdmin)

	if err != nil {
		logger.Error(err)
	} else {
		logger.Info("[application.go] Started using database: ",
			globals.Server.DbConfig.Name)

		// Give the option to alter database tables before migrating data
		DBAlterTables(logCtx, globals.Server.Db)

		// Migrate database tables
		DBMigrate(logCtx, globals.Server.Db)

		// Run custom DB migration scripts
		migrate.ToUniqueNamesWithForeignKeys(logCtx, globals.Server.Db)

		DBAddDefaultData(logCtx, globals.Server.Db)

		// Note: we populate DB with info only if not running `go test`
		if popPath, _ = ign.ReadEnvVar("IGN_POPULATE_PATH"); !isGoTest && popPath != "" {
			logger.Info("Using IGN_POPULATE_PATH with value: ", popPath)
			DBPopulate(logCtx, popPath, globals.Server.Db, true)
		}

		// After loading initial data, apply custom indexes. Eg: fulltext indexes
		DBAddCustomIndexes(logCtx, globals.Server.Db)

		// Initialize SubT database
		subt.Initialize(logCtx, globals.Server.Db)
		// Set SubT's default cloud implementation (S3)
		useAwsInTests := false
		awsBucketEnvVar := "AWS_BUCKET_PREFIX"
		if isGoTest {
			useStr, err := ign.ReadEnvVar("AWS_BUCKET_USE_IN_TESTS")
			if err == nil {
				flag, err2 := strconv.ParseBool(useStr)
				if err2 == nil {
					useAwsInTests = flag
				}
			}
			if useAwsInTests {
				awsBucketEnvVar += "_TEST"
			}
		}
		if !isGoTest || useAwsInTests {
			p, err := ign.ReadEnvVar(awsBucketEnvVar)
			if err != nil {
				panic("error reading " + awsBucketEnvVar)
			}
			subt.BucketServerImpl = subt.NewS3Bucket(p)
		}
	}

	// Set the default location to Collections (if missing).
	migrate.CollectionsSetDefaultLocation(logCtx, globals.Server.Db)
	// Reset Models/Worlds' Downloads and Likes counters, if needed.
	migrate.RecomputeDownloadsAndLikes(logCtx, globals.Server.Db)
	// Reset Models/Worlds' Zip File Sizes.
	migrate.RecomputeZipFileSizes(logCtx, globals.Server.Db)
	// Update resource tables (models/worlds) to be 'public' if not set.
	migrate.MakeResourcesPublicWhenNotSet(logCtx, globals.Server.Db)
	// Set casbin permissions for existing data
	migrate.CasbinPermissions(logCtx, globals.Server.Db)
	// Migrate competition score entries from logfile scores
	migrate.LogFileScoresToCompetitionScore(globals.Server.Db, "Tunnel Qualifiers")
	// Migrate logic
	migrate.ToModelGitRepositories(logCtx)
}

func initValidator() *validator.Validate {
	validate := validator.New()
	InstallCustomValidators(validate)
	return validate
}

/////////////////////////////////////////////////
// Run the router and server
func main() {
	globals.Server.Run()
}
