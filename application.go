// Package main Fuel Server REST API
//
// This package provides a REST API to the Fuel server.
//
// Schemes: https
// Host: fuel.gazebosim.org
// BasePath: /1.0
// Version: 0.1.0
// License: Apache 2.0
// Contact: info@openrobotics.org
//
// swagger:meta
// go:generate swagger generate spec -m
package main

// \todo Add in the following to the comments at the top of this file to enable
// security
//
// SecurityDefinitions:
//   token:
//     type: apiKey
//     name: authorization
//     in: header
//     description: Fuel token
//   auth0:
//     type: apiKey
//     name: authorization
//     in: header
//     description: Auth0 token. Note, It must start with 'Bearer '
//

// Import this file's dependencies
import (
	"github.com/gazebo-web/fuel-server/bootstrap"
	"github.com/gazebo-web/fuel-server/bundles/subt"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/migrate"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/go-playground/form"
	"github.com/joho/godotenv"
	"gopkg.in/go-playground/validator.v9"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Impl note: we move this as a constant as it is used by tests.
const sysAdminForTest = bootstrap.SysAdminForTest

// init initializes the config for the web fuel server.
//
// Environment variables:
//
//	IGN_DB_USERNAME  : Mysql username
//	IGN_DB_PASSWORD  : Mysql password
//	IGN_DB_ADDRESS   : Mysql address (host:port)
//	IGN_DB_NAME      : Mysql database name (such as "fuel")
//	IGN_FUEL_RESOURCE_DIR : Directory with all resources (models, worlds)
//	AUTH0_RSA256_PUBLIC_KEY   : Auth0 public RSA 256 key
func init() {
	// Load a .env file from the current working directory (or any of the
	// standard paths godotenv searches) into the process environment. This
	// runs before any gz.ReadEnvVar / os.Getenv calls below, so variables
	// defined in .env are visible to the rest of init. Silent no-op when
	// no .env file is present — production deployments continue to rely
	// on environment variables injected by the orchestrator.
	_ = godotenv.Load()

	var popPath string
	logger, logCtx := bootstrap.Logger("init")
	isGoTest := bootstrap.IsGoTest()

	bootstrap.QueryCache(logger, isGoTest)
	bootstrap.ResourceDir(isGoTest)
	bootstrap.Server(logger)

	// Create the main Router and set it to the server.
	// Note: here it is the place to define multiple APIs
	verifier := bootstrap.Verifier()
	mainRouter := gz.NewRouter()
	apiPrefix := "/" + globals.APIVersion
	r := mainRouter.PathPrefix(apiPrefix).Subrouter()
	bootstrap.ConfigureRoutes(verifier, apiPrefix, r, routes)

	// MCP server: registry gets its tools populated below after the rest
	// of the app state (globals.Permissions, globals.VCSRepoFactory, ...)
	// has been initialized. The registry is stored in a package-level
	// variable so the route handler can see it.
	initMCPServer(buildMCPRegistry())

	// Now create a sub router for SubT, enabled with /subt/
	subtPrefix := apiPrefix + "/subt"
	sub := mainRouter.PathPrefix(subtPrefix).Subrouter()
	bootstrap.ConfigureRoutes(verifier, subtPrefix, sub, subTRoutes)

	// Special swagger.json file server route
	swaggerRoute := "/" + globals.APIVersion + "/swagger.json"
	mainRouter.HandleFunc(swaggerRoute, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Access-Control-Allow-Methods",
			"GET, HEAD, POST, PUT, PATCH, DELETE")

		w.Header().Set("Access-Control-Allow-Credentials", "true")

		w.Header().Set("Access-Control-Allow-Headers",
			`Accept, Accept-Language, Content-Language, Origin,
                  Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token,
                  Authorization`)
		w.Header().Set("Access-Control-Allow-Origin", "*")

		w.Header().Set("Access-Control-Expose-Headers", "Link, X-Total-Count, X-Ign-Resource-Version")

		http.ServeFile(w, req, "swagger.json")
	})

	globals.Server.SetRouter(mainRouter)

	globals.FlagsEmailRecipient, _ = gz.ReadEnvVar("IGN_FLAGS_EMAIL_TO")
	globals.FlagsEmailSender, _ = gz.ReadEnvVar("IGN_FLAGS_EMAIL_FROM")
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

	bootstrap.VCS()

	globals.MaxCategoriesPerModel = 2
	if value, err := gz.ReadEnvVar("IGN_MAX_MODEL_CATEGORIES"); err == nil {
		if convertedValue, err := strconv.Atoi(value); err == nil {
			globals.MaxCategoriesPerModel = convertedValue
		}
	}

	// initialize permissions
	if err := bootstrap.Permissions(logger, isGoTest); err != nil {
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
		if popPath, _ = gz.ReadEnvVar("IGN_POPULATE_PATH"); !isGoTest && popPath != "" {
			logger.Info("Using IGN_POPULATE_PATH with value: ", popPath)
			DBPopulate(logCtx, popPath, globals.Server.Db, true)
		}

		// After loading initial data, apply custom indexes. Eg: fulltext indexes
		DBAddCustomIndexes(logCtx, globals.Server.Db)

		// Initialize SubT database
		subt.Initialize(logCtx, globals.Server.Db)
		// Set SubT's default cloud implementation (S3)
		bootstrap.S3(logger, isGoTest)
	}

	bootstrap.Storage()

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

	// Connect to ElasticSearch.
	_ = connectToElasticSearch(logCtx)
}

func initValidator() *validator.Validate {
	validate := validator.New()
	InstallCustomValidators(validate)
	return validate
}

// main runs the router and server
func main() {
	globals.Server.Run()
}
