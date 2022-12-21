package globals

import (
	"context"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/gazebo-web/fuel-server/permissions"
	"github.com/gazebo-web/fuel-server/vcs"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/go-playground/form"
	"gopkg.in/go-playground/validator.v9"
)

// TODO: remove as much as possible from globals

/////////////////////////////////////////////////
/// Define global variables here

// Server encapsulates database, router, and auth0
var Server *gz.Server

// APIVersion is route api version.
// See also routes and routers
// \todo: Add support for multiple versions.
var APIVersion = "1.0"

// ResourceDir is the directory where all resources are located.
var ResourceDir string

// Validate references the global structs validator.
// See https://github.com/go-playground/validator.
// We use a single instance of validator, as it caches struct info
var Validate *validator.Validate

// FormDecoder holds a reference to the global Form Decoder.
// See https://github.com/go-playground/form.
// We use a single instance of Decoder, as it caches struct info
var FormDecoder *form.Decoder

// FlagsEmailRecipient is the target email to use when sending
// flags notifications. It is set using IGN_FLAGS_EMAIL_TO env var.
var FlagsEmailRecipient string

// FlagsEmailSender is the sender email to use when sending
// flags notifications. It is set using IGN_FLAGS_EMAIL_FROM env var.
var FlagsEmailSender string

// LeaderboardOrganizationFilter contains a list of comma-separated
// organizations that will be excluded from leaderboard score results.
var LeaderboardOrganizationFilter []string

// LeaderboardCircuitFilter contains a list of comma-separated circuits that
// will be excluded from leaderboard score results.
var LeaderboardCircuitFilter []string

// VCSRepoFactory is the factory function used to create new
// repositories to manage versions of Models, Worlds, Plugins, etc.
// Our current implementation uses go-git.
var VCSRepoFactory (func(ctx context.Context, dirpath string) vcs.VCS)

// Permissions manages permissions for users, roles and resources.
var Permissions *permissions.Permissions

// MaxCategoriesPerModel defines the maximum amount of categories that can be assigned to a model
var MaxCategoriesPerModel int

// ElasticSearch is a pointer to the Elastic Search client.
var ElasticSearch *elasticsearch.Client
