package main

import (
	"github.com/gazebo-web/gz-go/v7"
)

////////////////////////////////////////////////
// Declare the routes. See also router.go
var routes = gz.Routes{

	////////////
	// Models //
	////////////

	// Route for all models
	gz.Route{
		Name:        "Models",
		Description: "Information about all models",
		URI:         "/models",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /models models listModels
			//
			// Get list of models.
			//
			// Get a list of models. Models will be returned paginated,
			// with pages of 20 models by default. The user can request a
			// different page with query parameter 'page', and the page size
			// can be defined with query parameter 'per_page'.
			// The route supports the 'order' parameter, with values 'asc' and
			// 'desc' (default: desc).
			// It also supports the 'q' parameter to perform a fulltext search on models
			// name, description and tags.
			//
			//   Produces:
			//   - application/json
			//   - application/x-protobuf
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: jsonModels
			gz.Method{
				Type:        "GET",
				Description: "Get all models",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONListResult("Models", SearchHandler(ModelList))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(SearchHandler(ModelList))},
					gz.FormatHandler{Handler: gz.JSONListResult("Models", SearchHandler(ModelList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /models models createModel
			//
			// Create model
			//
			// Creates a new model. The request body should contain the
			// following fields: 'modelName', 'urlName', 'description',
			// 'license' (number), 'permission' (number). All values as strings.
			// 'tags': a string containing a comma separated list of tags.
			// The model owner will be retrieved from the passed JWT.
			// 'file': multiple files in the multipart form.
			//
			//   Consumes:
			//   - multipart/form-data
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbModel
			gz.Method{
				Type:        "POST",
				Description: "Create a new model",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(ModelCreate)},
				},
			},
		},
	},

	// Route that returns a list of models from a team/user (ie. an 'owner')
	gz.Route{
		Name:        "OwnerModels",
		Description: "Information about models belonging to an owner. The {username} URI option will limit the scope to the specified user/team. Otherwise all models are considered.",
		URI:         "/{username}/models",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/models models listOwnerModels
			//
			// Get owner's models
			//
			// Get a list of models for the specified owner.
			// Models will be returned paginated,
			// with pages of 20 models by default. The user can request a
			// different page with query parameter 'page' (first page is value 1).
			// The page size can be controlled with query parameter 'per_page',
			// with a maximum of 100 items per page.
			// The route supports the 'order' parameter, with values 'asc' and
			// 'desc' (default: desc).
			// It also supports the 'q' parameter to perform a fulltext search on models
			// name, description and tags.
			//
			//   Produces:
			//   - application/json
			//   - application/x-protobuf
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: jsonModels
			gz.Method{
				Type:        "GET",
				Description: "Get all models of the specified team/user",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONListResult("Models", SearchHandler(ModelList))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(SearchHandler(ModelList))},
					gz.FormatHandler{Handler: gz.JSONListResult("Models", SearchHandler(ModelList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that handles likes to a model from an owner
	gz.Route{
		Name:        "ModelLikes",
		Description: "Handles the likes of a model.",
		URI:         "/{username}/models/{model}/likes",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/models/{model}/likes models modelLikeCreate
			//
			// Like a model
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Model
			gz.Method{
				Type:        "POST",
				Description: "Like a model",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("model", true, ModelOwnerLikeCreate)))},
				},
			},
			// swagger:route DELETE /{username}/models/{model}/likes models modelUnlike
			//
			// Unlike a model
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Model
			gz.Method{
				Type:        "DELETE",
				Description: "Unlike a model",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("model", true, ModelOwnerLikeRemove)))},
				},
			},
		},
	},

	// Route that returns a list of models liked by a user.
	gz.Route{
		Name:        "ModelLikeList",
		Description: "Models liked by a user.",
		URI:         "/{username}/likes/models",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/likes/models models modelLikeList
			//
			// Get models liked by a user.
			//
			// Get a list of models liked by the specified user.
			// Models will be returned paginated, with pages of 20 models by default.
			// The user can request a different page with query parameter 'page' (first page is value 1).
			// The page size can be controlled with query parameter 'per_page', with a maximum of
			// 100 items per page.
			// The route supports the 'order' parameter, with values 'asc' and 'desc' (default: desc).
			// It also supports the 'q' parameter to perform a fulltext search on models name,
			// description and tags.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: jsonModels
			gz.Method{
				Type:        "GET",
				Description: "Get all models liked by the specified user",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONListResult("Models", SearchHandler(ModelLikeList))},
					gz.FormatHandler{Handler: gz.JSONListResult("Models", SearchHandler(ModelLikeList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that returns the files tree of a single model based on owner, model name, and version
	gz.Route{
		Name:        "ModelOwnerVersionFileTree",
		Description: "Route that returns the files tree of a single model.",
		URI:         "/{username}/models/{model}/{version}/files",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/models/{model}/{version}/files models modelFileTree
			//
			// Model's file tree.
			//
			// Return the files information of a given model.
			//
			//   Produces:
			//   - application/json
			//   - application/x-protobuf
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: ModelFileTree
			gz.Method{
				Type:        "GET",
				Description: "Get file tree",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameOwnerHandler("model", false, ModelOwnerVersionFileTree))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(NameOwnerHandler("model", false, ModelOwnerVersionFileTree))},
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("model", false, ModelOwnerVersionFileTree))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that downloads an individual file from a model based on owner, model name, and version
	gz.Route{
		Name:        "ModelOwnerVersionIndividualFileDownload",
		Description: "Download individual file from a model.",
		URI:         "/{username}/models/{model}/{version}/files/{path:.+}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/models/{model}/{version}/files/{path} models downloadModelFile
			//
			// Download an individual file from a model.
			//
			//   Produces:
			//   - application/octet-stream
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: fileResponse
			gz.Method{
				Type:        "GET",
				Description: "GET a file",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("model", false, ModelOwnerVersionIndividualFileDownload)))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that returns a model, by name, from a team/user
	gz.Route{
		Name:        "OwnerModelIndex",
		Description: "Information about a model belonging to an owner.",
		URI:         "/{username}/models/{model}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/models/{model} models singleOwnerModel
			//
			// Get a single model from an owner
			//
			// Return a model given its owner and name.
			//
			//   Produces:
			//   - application/json
			//   - application/x-protobuf
			//   - application/zip
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Model
			gz.Method{
				Type:        "GET",
				Description: "Get a model belonging to the specified team/user",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
					gz.FormatHandler{Extension: ".zip", Handler: gz.Handler(NoResult(NameOwnerHandler("model", false, ModelOwnerVersionZip)))},
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route PATCH /{username}/models/{model} models modelUpdate
			//
			// Update a model
			//
			// Update a model
			//
			//   Consumes:
			//   - multipart/form-data
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Model
			gz.Method{
				Type:        "PATCH",
				Description: "Edit a model",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("model", true, ModelUpdate))},
				},
			},
			// swagger:route DELETE /{username}/models/{model} models deleteModel
			//
			// Delete a model
			//
			// Deletes a model given its owner and name.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			gz.Method{
				Type:        "DELETE",
				Description: "Deletes a single model",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("model", true, ModelOwnerRemove)))},
				},
			},
		},
	},

	// Route that transfers a model
	gz.Route{
		Name:        "OwnerModelIndex",
		Description: "Transfer a model to another owner.",
		URI:         "/{username}/models/{model}/transfer",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/models/{model}/transfer models modelTransfer
			//
			// Transfer a model
			//
			//   Consumes:
			//   - multipart/form-data
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Model
			gz.Method{
				Type:        "POST",
				Description: "Transfer a model",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("model", true, ModelTransfer))},
				},
			},
		},
	},

	// Route that returns a model zip file from a team/user
	gz.Route{
		Name:        "OwnerModelVersion",
		Description: "Download a versioned model zip file belonging to an owner.",
		URI:         "/{username}/models/{model}/{version}/{model}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/models/{model}/{version}/{model} models singleOwnerModel
			//
			// Get a single model zip file from an owner
			//
			// Return a model zip file given its owner, name, and version.
			//
			//   Produces:
			//   - application/zip
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Model
			gz.Method{
				Type:        "GET",
				Description: "Get a model of specified version belonging to the specified team/user",
				// Format handlers
				// if empty file extension is given, it returns model's meta data
				// and {version} is then ignored
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".zip", Handler: gz.Handler(NoResult(NameOwnerHandler("model", false, ModelOwnerVersionZip)))},
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that clones a model
	gz.Route{
		Name:        "CloneModel",
		Description: "Clone a model",
		URI:         "/{username}/models/{model}/clone",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/models/{model}/clone models cloneModel
			//
			// Clones a models
			//
			// Clones a model.
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OK
			gz.Method{
				Type:        "POST",
				Description: "Clones a model",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("model", false, ModelClone))},
				},
			},
		},
	},

	// Route that handles model reports
	gz.Route{
		Name:        "ReportModel",
		Description: "Report a model",
		URI:         "/{username}/models/{model}/report",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route POST /{username}/models/{model}/report models reportModel
			//
			// Reports a model.
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OK
			gz.Method{
				Type:        "POST",
				Description: "Reports a model",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("model", false, ReportModelCreate)))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	////////////
	// Worlds //
	////////////

	// Route for all worlds
	gz.Route{
		Name:        "Worlds",
		Description: "Information about all worlds",
		URI:         "/worlds",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /worlds worlds listWorlds
			//
			// Get list of worlds.
			//
			// Get a list of worlds. Worlds will be returned paginated,
			// with pages of 20 worlds by default. The user can request a
			// different page with query parameter 'page', and the page size
			// can be defined with query parameter 'per_page'.
			// The route supports the 'order' parameter, with values 'asc' and
			// 'desc' (default: desc).
			// It also supports the 'q' parameter to perform a fulltext search on worlds
			// name, description and tags.
			//
			//   Produces:
			//   - application/json
			//   - application/x-protobuf
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: jsonWorlds
			gz.Method{
				Type:        "GET",
				Description: "Get all worlds",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONListResult("Worlds", SearchHandler(WorldList))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(SearchHandler(WorldList))},
					gz.FormatHandler{Handler: gz.JSONListResult("Worlds", SearchHandler(WorldList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /worlds worlds createWorld
			//
			// Create world
			//
			// Creates a new world. The request body should contain the
			// following fields: 'name', 'description',
			// 'license' (number), 'permission' (number). All values as strings.
			// 'tags': a string containing a comma separated list of tags.
			// The worlds owner will be retrieved from the passed JWT.
			// 'file': multiple files in the multipart form.
			//
			//   Consumes:
			//   - multipart/form-data
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbWorld
			gz.Method{
				Type:        "POST",
				Description: "Create a new world",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(WorldCreate)},
				},
			},
		},
	},

	// Route that returns a list of worlds from a team/user (ie. an 'owner')
	gz.Route{
		Name:        "OwnerWorlds",
		Description: "Information about worlds belonging to an owner. The {username} URI option will limit the scope to the specified user/team. Otherwise all worlds are considered.",
		URI:         "/{username}/worlds",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/worlds worlds listOwnerWorlds
			//
			// Get owner's worlds
			//
			// Get a list of worlds for the specified owner.
			// Worlds will be returned paginated,
			// with pages of 20 worlds by default. The user can request a
			// different page with query parameter 'page' (first page is value 1).
			// The page size can be controlled with query parameter 'per_page',
			// with a maximum of 100 items per page.
			// The route supports the 'order' parameter, with values 'asc' and
			// 'desc' (default: desc).
			// It also supports the 'q' parameter to perform a fulltext search on worlds
			// name, description and tags.
			//
			//   Produces:
			//   - application/json
			//   - application/x-protobuf
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: jsonWorlds
			gz.Method{
				Type:        "GET",
				Description: "Get all worlds of the specified team/user",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONListResult("Worlds", SearchHandler(WorldList))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(SearchHandler(WorldList))},
					gz.FormatHandler{Handler: gz.JSONListResult("Worlds", SearchHandler(WorldList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that handles likes to a world from an owner
	gz.Route{
		Name:        "WorldLikes",
		Description: "Handles the likes of a world.",
		URI:         "/{username}/worlds/{world}/likes",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/worlds/{world}/likes worlds worldLikeCreate
			//
			// Like a world
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: World
			gz.Method{
				Type:        "POST",
				Description: "Like a world",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("world", true, WorldLikeCreate)))},
				},
			},
			// swagger:route DELETE /{username}/worlds/{world}/likes worlds worldUnlike
			//
			// Unlike a world
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: World
			gz.Method{
				Type:        "DELETE",
				Description: "Unlike a world",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("world", true, WorldLikeRemove)))},
				},
			},
		},
	},

	// Route that returns a list of worlds liked by a user.
	gz.Route{
		Name:        "WorldLikeList",
		Description: "Worlds liked by a user.",
		URI:         "/{username}/likes/worlds",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/likes/worlds worlds worldLikeList
			//
			// Get worlds liked by a user.
			//
			// Get a list of worlds liked by the specified user.
			// Worlds will be returned paginated, with pages of 20 worlds by default.
			// The user can request a different page with query parameter 'page' (first page is value 1).
			// The page size can be controlled with query parameter 'per_page', with a maximum of
			// 100 items per page.
			// The route supports the 'order' parameter, with values 'asc' and 'desc' (default: desc).
			// It also supports the 'q' parameter to perform a fulltext search on world's name,
			// description and tags.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: jsonWorlds
			gz.Method{
				Type:        "GET",
				Description: "Get all worlds liked by the specified user",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONListResult("Worlds", SearchHandler(WorldLikeList))},
					gz.FormatHandler{Handler: gz.JSONListResult("Worlds", SearchHandler(WorldLikeList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that returns the files tree of a single world based on owner, name, and version
	gz.Route{
		Name:        "WorldFileTree",
		Description: "Route that returns the files tree of a single world.",
		URI:         "/{username}/worlds/{world}/{version}/files",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/worlds/{world}/{version}/files worlds worldFileTree
			//
			// World's file tree.
			//
			// Return the files information of a given world.
			//
			//   Produces:
			//   - application/json
			//   - application/x-protobuf
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: WorldFileTree
			gz.Method{
				Type:        "GET",
				Description: "Get file tree",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameOwnerHandler("world", false, WorldFileTree))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(NameOwnerHandler("world", false, WorldFileTree))},
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("world", false, WorldFileTree))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that downloads an individual file from a world based on owner, name, and version
	gz.Route{
		Name:        "WorldIndividualFileDownload",
		Description: "Download individual file from a world.",
		URI:         "/{username}/worlds/{world}/{version}/files/{path:.+}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/worlds/{world}/{version}/files/{path} worlds downloadWorldFile
			//
			// Download an individual file from a world.
			//
			//   Produces:
			//   - application/octet-stream
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: fileResponse
			gz.Method{
				Type:        "GET",
				Description: "GET a file",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("world", false, WorldIndividualFileDownload)))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that returns a world, by name, from a team/user
	gz.Route{
		Name:        "WorldIndex",
		Description: "Information about a world belonging to an owner.",
		URI:         "/{username}/worlds/{world}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/worlds/{world} worlds singleOwnerWorld
			//
			// Get a single world from an owner
			//
			// Return a world given its owner and name.
			//
			//   Produces:
			//   - application/json
			//   - application/x-protobuf
			//   - application/zip
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: World
			gz.Method{
				Type:        "GET",
				Description: "Get a world belonging to the specified team/user",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameOwnerHandler("world", false, WorldIndex))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(NameOwnerHandler("world", false, WorldIndex))},
					gz.FormatHandler{Extension: ".zip", Handler: gz.Handler(NoResult(NameOwnerHandler("world", false, WorldZip)))},
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("world", false, WorldIndex))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route PATCH /{username}/worlds/{world} worlds worldUpdate
			//
			// Update a world
			//
			// Update a world
			//
			//   Consumes:
			//   - multipart/form-data
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: World
			gz.Method{
				Type:        "PATCH",
				Description: "Edit a world",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("world", true, WorldUpdate))},
				},
			},
			// swagger:route DELETE /{username}/worlds/{world} world deleteWorld
			//
			// Delete a world
			//
			// Deletes a world given its owner and name.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			gz.Method{
				Type:        "DELETE",
				Description: "Deletes a single world",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("world", true, WorldRemove)))},
				},
			},
		},
	},

	// Route that transfers a world
	gz.Route{
		Name:        "OwnerWorldTransfer",
		Description: "Transfer a world to another owner.",
		URI:         "/{username}/worlds/{world}/transfer",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/worlds/{world}/transfer models worldTransfer
			//
			// Transfer a world
			//
			//   Consumes:
			//   - multipart/form-data
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Model
			gz.Method{
				Type:        "POST",
				Description: "Transfer a world",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("world", true, WorldTransfer))},
				},
			},
		},
	},

	// Route that returns a world zip file from a team/user
	gz.Route{
		Name:        "WorldVersion",
		Description: "Download a versioned world zip file belonging to an owner.",
		URI:         "/{username}/worlds/{world}/{version}/{world}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/worlds/{world}/{version}/{world} worlds singleOwnerWorld
			//
			// Get a single world zip file from an owner
			//
			// Return a world zip file given its owner, name, and version.
			//
			//   Produces:
			//   - application/zip
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: World
			gz.Method{
				Type:        "GET",
				Description: "Get a world of specified version belonging to the specified team/user",
				// Format handlers
				// if empty file extension is given, it returns world's meta data
				// and {version} is then ignored
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".zip", Handler: gz.Handler(NoResult(NameOwnerHandler("world", false, WorldZip)))},
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("world", false, WorldIndex))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that clones a world
	gz.Route{
		Name:        "CloneWorld",
		Description: "Clone a world",
		URI:         "/{username}/worlds/{world}/clone",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/worlds/{world}/clone worlds cloneWorld
			//
			// Clones a world
			//
			// Clones a world.
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OK
			gz.Method{
				Type:        "POST",
				Description: "Clones a world",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("world", false, WorldClone))},
				},
			},
		},
	},

	// Route that handles world reports
	gz.Route{
		Name:        "ReportWorld",
		Description: "Report a world",
		URI:         "/{username}/worlds/{world}/report",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route POST /{username}/worlds/{world}/report worlds reportWorld
			//
			// Reports a world.
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OK
			gz.Method{
				Type:        "POST",
				Description: "Reports a world",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("world", false, ReportWorldCreate)))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that returns the modelIncludes of a world.
	gz.Route{
		Name:        "WorldModelIncludes",
		Description: "Route that returns the external models referenced by a world",
		URI:         "/{username}/worlds/{world}/{version}/{world}/modelrefs",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/worlds/{world}/{version}/{world}/modelrefs worlds worldModelIncludes
			//
			// World's model references.
			//
			// Return the external models referenced by a world.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: ModelIncludes
			gz.Method{
				Type:        "GET",
				Description: "World's ModelIncludes ",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameOwnerHandler("world", false, WorldModelReferences))},
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("world", false, WorldModelReferences))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	/////////////////
	// Collections //
	/////////////////

	// Route for all Collections
	gz.Route{
		Name:        "Collection",
		Description: "Information about all collections",
		URI:         "/collections",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /collections collections listCollections
			//
			// Get list of collections.
			//
			// Get a list of collections. Collections will be returned paginated,
			// with pages of 20 items by default. The user can request a
			// different page with query parameter 'page', and the page size
			// can be defined with query parameter 'per_page'.
			// The route supports the 'order' parameter, with values 'asc' and
			// 'desc' (default: desc).
			// It also supports the 'q' parameter to perform a fulltext search on
			// name and description.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbCollections
			gz.Method{
				Type:        "GET",
				Description: "Get all collections",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(SearchHandler(CollectionList))},
					gz.FormatHandler{Handler: gz.JSONResult(SearchHandler(CollectionList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /collections collections createCollection
			//
			// Create collection
			//
			// Creates a new collection. The request body should contain the
			// following fields: 'name', 'description'. All values as strings.
			// The collection owner will be retrieved from the passed JWT.
			//
			//   Consumes:
			//   - multipart/form-data
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbCollection
			gz.Method{
				Type:        "POST",
				Description: "Create a new collection",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(CollectionCreate)},
				},
			},
		},
	},

	// Route that returns a list of collections from a team/user (ie. an 'owner')
	gz.Route{
		Name: "OwnerCollections",
		Description: "Information about worlds belonging to an owner. The {username} URI option " +
			"will limit the scope to the specified user/team. Otherwise all collections are considered.",
		URI:     "/{username}/collections",
		Headers: gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/collections collections listOwnerCollections
			//
			// Get owner's collections
			//
			// Get a list of collections for the specified owner.
			// Collections will be returned paginated,
			// with pages of 20 items by default. The user can request a
			// different page with query parameter 'page' (first page is value 1).
			// The page size can be controlled with query parameter 'per_page',
			// with a maximum of 10belonging0 items per page.
			// The route supports the 'order' parameter, with values 'asc' and
			// 'desc' (default: desc).
			// It also supports the 'q' parameter to perform a fulltext search on
			// name and description.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbCollections
			gz.Method{
				Type:        "GET",
				Description: "Get all collections of the specified team/user",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(SearchHandler(CollectionList))},
					gz.FormatHandler{Handler: gz.JSONResult(SearchHandler(CollectionList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that returns a Collection, by name, from a team/user
	gz.Route{
		Name:        "CollectionIndex",
		Description: "Information about a collection belonging to an owner.",
		URI:         "/{username}/collections/{collection}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/collections/{collection} collections singleOwnerCollection
			//
			// Get a single collection from an owner
			//
			// Return a collection given its owner and name.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbCollection
			gz.Method{
				Type:        "GET",
				Description: "Get a collection belonging to the specified team/user",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameOwnerHandler("collection", false, CollectionIndex))},
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("collection", false, CollectionIndex))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route PATCH /{username}/collections/{collection} collections collectionUpdate
			//
			// Update a collection
			//
			// Update a collection
			//
			//   Consumes:
			//   - multipart/form-data
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbCollection
			gz.Method{
				Type:        "PATCH",
				Description: "Edit a collection",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("collection", true, CollectionUpdate))},
				},
			},
			// swagger:route DELETE /{username}/collections/{collection} collection deleteCollection
			//
			// Delete a Collection
			//
			// Deletes a Collection given its owner and name.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OK
			gz.Method{
				Type:        "DELETE",
				Description: "Deletes a single collection",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("collection", true, CollectionRemove)))},
				},
			},
		},
	},

	gz.Route{
		Name:        "OwnerCollectionTransfer",
		Description: "Transfer a collection to another owner.",
		URI:         "/{username}/collections/{collection}/transfer",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/collections/{collection}/transfer collections collectionTransfer
			//
			// Transfer a collection
			//
			//   Consumes:
			//   - multipart/form-data
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Model
			gz.Method{
				Type:        "POST",
				Description: "Transfer a collection",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("collection", true, CollectionTransfer))},
				},
			},
		},
	},
	// Route that clones a collection
	gz.Route{
		Name:        "CloneCollection",
		Description: "Clone a collection",
		URI:         "/{username}/collections/{collection}/clone",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/collections/{collection}/clone collections cloneCollection
			//
			// Clones a collection
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OK
			gz.Method{
				Type:        "POST",
				Description: "Clones a collection",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("collection", false, CollectionClone))},
				},
			},
		},
	},

	// Route that downloads an individual file from a collection.
	// It is used to download the collection logo and banner.
	gz.Route{
		Name:        "CollectionIndividualFileDownload",
		Description: "Download individual file from a collection.",
		URI:         "/{username}/collections/{collection}/{version}/files/{path:.+}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/collections/{collection}/{version}/files/{path} collections downloadColFile
			//
			// Download an individual file from a collection.
			//
			//   Produces:
			//   - application/octet-stream
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: fileResponse
			gz.Method{
				Type:        "GET",
				Description: "GET a file",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("collection", false, CollectionIndividualFileDownload)))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route to list, add and remove models from collections.
	gz.Route{
		Name:        "CollectionModels",
		Description: "Information about models from a collection",
		URI:         "/{username}/collections/{collection}/models",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/collections/{collection}/models collections collectionModels
			//
			// Lists the models of a collection
			//
			// Return the list of models that belong to a collection
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbCollectionAssets
			gz.Method{
				Type:        "GET",
				Description: "Get the models associated to a collection",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONListResult("Models", NameOwnerHandler("collection", false, CollectionModelsList))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(NameOwnerHandler("collection", false, CollectionModelsList))},
					gz.FormatHandler{Handler: gz.JSONListResult("Models", NameOwnerHandler("collection", false, CollectionModelsList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/collections/{collection}/models collections collectionModelAdd
			//
			// Add a model to a collection
			//
			// Adds a model to a collection
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OK
			gz.Method{
				Type:        "POST",
				Description: "Add a model to a collection",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("collection", true, CollectionModelAdd)))},
				},
			},
			// swagger:route DELETE /{username}/collections/{collection}/models collection collectionModelRemove
			//
			// Remove model from Collection
			//
			// Removes a model from a Collection
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OK
			gz.Method{
				Type:        "DELETE",
				Description: "Removes a model from a collection",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("collection", true, CollectionModelRemove)))},
				},
			},
		},
	},

	// Route to list, add and remove worlds from collections.
	gz.Route{
		Name:        "CollectionWorlds",
		Description: "Information about Worlds from a collection",
		URI:         "/{username}/collections/{collection}/worlds",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/collections/{collection}/worlds collections collectionWorlds
			//
			// Lists the worlds of a collection
			//
			// Return the list of worlds that belong to a collection
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbCollectionAssets
			gz.Method{
				Type:        "GET",
				Description: "Get the worlds associated to a collection",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONListResult("Worlds", NameOwnerHandler("collection", false, CollectionWorldsList))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(NameOwnerHandler("collection", false, CollectionWorldsList))},
					gz.FormatHandler{Handler: gz.JSONListResult("Worlds", NameOwnerHandler("collection", false, CollectionWorldsList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/collections/{collection}/worlds collections collectionWorldAdd
			//
			// Add a world to a collection
			//
			// Adds a world to a collection
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OK
			gz.Method{
				Type:        "POST",
				Description: "Add a world to a collection",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("collection", true, CollectionWorldAdd)))},
				},
			},
			// swagger:route DELETE /{username}/collections/{collection}/worlds collection collectionWorldRemove
			//
			// Remove world from Collection
			//
			// Removes a world from a Collection
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OK
			gz.Method{
				Type:        "DELETE",
				Description: "Removes a world from a collection",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.Handler(NoResult(NameOwnerHandler("collection", true, CollectionWorldRemove)))},
				},
			},
		},
	},

	// Route that returns the list of collections associated to a model
	gz.Route{
		Name:        "ModelCollections",
		Description: "List of collections associated to a model.",
		URI:         "/{username}/models/{model}/collections",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/models/{model}/collections collections modelCollections
			//
			// List of collections associated to a model.
			//
			// List of collections associated to a model.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbCollections
			gz.Method{
				Type:        "GET",
				Description: "List of collections associated to a model",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameOwnerHandler("model", false, ModelCollections))},
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("model", false, ModelCollections))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	// Route that returns the list of collections associated to a world
	gz.Route{
		Name:        "WorldCollections",
		Description: "List of collections associated to a world.",
		URI:         "/{username}/worlds/{world}/collections",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/worlds/{world}/collections collections worldCollections
			//
			// List of collections associated to a world.
			//
			// List of collections associated to a world.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: dbCollections
			gz.Method{
				Type:        "GET",
				Description: "List of collections associated to a world",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameOwnerHandler("world", false, WorldCollections))},
					gz.FormatHandler{Handler: gz.JSONResult(NameOwnerHandler("world", false, WorldCollections))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	///////////
	// Users //
	///////////

	// Route that returns login information for a given JWT
	gz.Route{
		Name:        "Login",
		Description: "Login a user",
		URI:         "/login",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /login users loginUser
			//
			// Login user
			//
			// Returns information about the user associated with the given JWT.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: UserResponse
			gz.Method{
				Type:        "GET",
				Description: "Login a user",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(Login)},
				},
			},
		},
	},

	// Route that returns information about all users
	gz.Route{
		Name:        "Users",
		Description: "Route for all users",
		URI:         "/users",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /users users listUsers
			//
			// Get a list of users. Access limited to administrators.
			//
			// Returns a paginated list of users,
			// with pages of 20 users by default. Only system administrators can
			// access this route. The administrator can request a
			// different page with query parameter 'page' (first page is value 1).
			// The page size can be controlled with query parameter 'per_page',
			// with a maximum of 100 items per page.
			//
			//   Parameters:
			//   + name: Private-Token
			//     description: A personal access token.
			//     in: header
			//     required: true
			//     type: string
			//   + name: page
			//     description: Request a specific page of users.
			//     in: query
			//     required: false
			//     type: integer
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: UserResponses
			gz.Method{
				Type:        "GET",
				Description: "Get all users information",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(PaginationHandler(UserList))},
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandler(UserList))},
				},
			},
			// swagger:route POST /users users createUser
			//
			// Create user
			//
			// Creates a new user. Note: the user identity will be retrieved from the passed JWT.
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: UserResponse
			gz.Method{
				Type:        "POST",
				Description: "Create a new user",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(UserCreate)},
				},
			},
		},
	},

	// Route that returns information about a user
	gz.Route{
		Name:        "UserIndex",
		Description: "Access information about a single user.",
		URI:         "/users/{username}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /users/{username} users singleUser
			//
			// Get a user
			//
			// Return a user given its username and a valid JWT.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: UserResponse
			gz.Method{
				Type:        "GET",
				Description: "Get user information",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameHandler("username", false, UserIndex))},
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("username", false, UserIndex))},
				},
			},
		},

		SecureMethods: gz.SecureMethods{
			// swagger:route DELETE /users/{username} users deleteUser
			//
			// Delete a user
			//
			// Deletes a user given its username and a valid JWT.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			gz.Method{
				Type:        "DELETE",
				Description: "Remove a user",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("username", true, UserRemove))},
				},
			},

			// swagger:route PATCH /users/{username} users updateUser
			//
			// Update a user
			//
			// Updates a user given its username and a valid JWT.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: UserResponse
			gz.Method{
				Type:        "PATCH",
				Description: "Update a user",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("username", true, UserUpdate))},
				},
			},
		},
	},

	// Routes to get and create access tokens.
	gz.Route{
		Name:        "AccessTokens",
		Description: "Routes to get and create access tokens.",
		URI:         "/users/{username}/access-tokens",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},

		SecureMethods: gz.SecureMethods{
			// swagger:route GET /users/{username}/access-tokens users getAccessToken
			//
			// Get the acccess tokens for a user.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			gz.Method{
				Type:        "GET",
				Description: "Get a user's access tokens",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandlerWithUser(AccessTokenList, true))},
				},
			},

			// swagger:route POST /users/{username}/access-tokens users createAccessToken
			//
			// Creates an access token.
			//
			// Creates an access token for a user.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			gz.Method{
				Type:        "POST",
				Description: "Create an access token",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("username", true, AccessTokenCreate))},
				},
			},
		},
	},

	// Routes to revoke access tokens
	gz.Route{
		Name:        "AccessTokens",
		Description: "Route to revoke access tokens.",
		URI:         "/users/{username}/access-tokens/revoke",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},

		SecureMethods: gz.SecureMethods{
			// swagger:route POST /users/{username}/access-tokens/revoke users revokeAccessToken
			//
			// Delete an acccess token that belongs to a user.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			gz.Method{
				Type:        "POST",
				Description: "Delete a user's access token",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("username", true, AccessTokenDelete))},
				},
			},
		},
	},

	// Route that returns the details of a single user or organization
	gz.Route{
		Name:        "OwnerProfile",
		Description: "Access the details of a single user OR organization.",
		URI:         "/profile/{username}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /profile/{username} users ownerProfile
			//
			// Get the profile of an owner
			//
			// Get the profile of an owner
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OwnerProfile
			gz.Method{
				Type:        "GET",
				Description: "Get profile information",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameHandler("username", false, OwnerProfile))},
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("username", false, OwnerProfile))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	//////////////
	// Licenses //
	//////////////

	// Route that returns information about all available licenses
	gz.Route{
		Name:        "Licenses",
		Description: "Route for all licenses",
		URI:         "/licenses",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /licenses licenses listLicenses
			//
			// List licenses
			//
			// Get the list of licenses. Licenses will be returned paginated,
			// with pages of 20 items by default. The user can request a
			// different page with query parameter 'page' (first page is value 1).
			// The page size can be controlled with query parameter 'per_page',
			// with a maximum of 100 items per page.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Licenses
			gz.Method{
				Type:        "GET",
				Description: "Get all licenses",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(PaginationHandler(LicenseList))},
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandler(LicenseList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},

	//////////////
	// Categories //
	//////////////

	// Categories route with slug
	// PATCH:
	gz.Route{
		Name:        "Categories",
		Description: "Routes for categories with slug",
		URI:         "/categories/{slug}",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			gz.Method{
				Type:        "PATCH",
				Description: "Update a category",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{
						Extension: "",
						Handler:   gz.JSONResult(CategoryUpdate),
					},
				},
			},
			gz.Method{
				Type:        "DELETE",
				Description: "Delete a category",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{
						Extension: "",
						Handler:   gz.JSONResult(CategoryDelete),
					},
				},
			},
		},
	},

	// Categories route
	// GET: Get the list of categories
	// POST: Create a new category
	gz.Route{
		Name:        "Categories",
		Description: "Route for categories",
		URI:         "/categories",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			gz.Method{
				Type:        "GET",
				Description: "Get all categories",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{
						Extension: ".json",
						Handler:   gz.JSONResult(CategoryList),
					},
					gz.FormatHandler{
						Extension: "",
						Handler:   gz.JSONResult(CategoryList),
					},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			gz.Method{
				Type:        "POST",
				Description: "Create a new category",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{
						Extension: "",
						Handler:   gz.JSONResult(CategoryCreate),
					},
				},
			},
		},
	},

	///////////////////
	// Organizations //
	///////////////////

	// Route that returns information about all organizations
	gz.Route{
		Name:        "Organizations",
		Description: "Route for all organizations",
		URI:         "/organizations",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /organizations organizations listOrganizations
			//
			// List organizations
			//
			// Get the list of organizations. Organizations will be returned paginated,
			// with pages of 20 organizations by default. The user can request a
			// different page with query parameter 'page' (first page is value 1).
			// The page size can be controlled with query parameter 'per_page',
			// with a maximum of 100 items per page.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OrganizationResponses
			gz.Method{
				Type:        "GET",
				Description: "Get all organizations information",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(PaginationHandler(OrganizationList))},
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandler(OrganizationList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /organizations organizations createOrganization
			//
			// Create organization
			//
			// Creates a new organization. Note: the user identity will be retrieved from the passed JWT.
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OrganizationResponse
			gz.Method{
				Type:        "POST",
				Description: "Create a new organization",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(OrganizationCreate)},
				},
			},
		},
	},

	// Route that returns information about an organization
	gz.Route{
		Name:        "OrganizationIndex",
		Description: "Access information about a single organization.",
		URI:         "/organizations/{name}",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /organizations/{name} organizations singleOrganization
			//
			// Get an organization
			//
			// Return an organization given its name and a valid JWT.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OrganizationResponse
			gz.Method{
				Type:        "GET",
				Description: "Get organization information",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameHandler("name", false, OrganizationIndex))},
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", false, OrganizationIndex))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route DELETE /organizations/{name} organizations deleteOrganizations
			//
			// Delete an organization
			//
			// Deletes an organization given its name and a valid JWT.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OrganizationResponse
			gz.Method{
				Type:        "DELETE",
				Description: "Remove an organization",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, OrganizationRemove))},
				},
			},
			// swagger:route PATCH /organizations/{name} organizations organizationUpdate
			//
			// Update an organization
			//
			// Update an organization
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: OrganizationResponse
			gz.Method{
				Type:        "PATCH",
				Description: "Edit an organization",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, OrganizationUpdate))},
				},
			},
		},
	},
	// Route that returns information about organization users
	gz.Route{
		Name:        "OrganizationUsers",
		Description: "Base route to list of users of an Organization",
		URI:         "/organizations/{name}/users",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /organizations/{name}/users organizations orgUsers
			//
			// Get the list of users of an organization
			//
			// Return the list of users of an organization.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: UserResponses
			gz.Method{
				Type:        "GET",
				Description: "Get the list of users of an organization",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(PaginationHandler(OrganizationUserList))},
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandler(OrganizationUserList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /organizations/{name}/users organizations addUserToOrganization
			//
			// Adds a user to an organization
			//
			// Adds a user to an organization.
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: UserResponse
			gz.Method{
				Type:        "POST",
				Description: "Adds a user to an Organization",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, OrganizationUserCreate))},
				},
			},
		},
	},
	// Route that returns information about organization users
	gz.Route{
		Name:        "OrganizationUserUpdate",
		Description: "Route to update and delete a member of an organization",
		URI:         "/organizations/{name}/users/{username}",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route DELETE /organizations/{name}/users/{username} organizations orgUserDelete
			//
			// Removes a user from an organization
			//
			// Removes a user from an organization
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: UserResponse
			gz.Method{
				Type:        "DELETE",
				Description: "Removes a user from an organization",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, OrganizationUserRemove))},
				},
			},
		},
	},

	// Route that returns information about organization teams
	gz.Route{
		Name:        "OrganizationTeams",
		Description: "Base route to list of teams of an Organization",
		URI:         "/organizations/{name}/teams",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /organizations/{name}/teams organizations orgTeams
			//
			// Get the list of teams of an organization
			//
			// Return the list of teams of an organization.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: TeamResponses
			gz.Method{
				Type:        "GET",
				Description: "Get the list of teams of an organization",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(PaginationHandler(OrganizationTeamsList))},
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandler(OrganizationTeamsList))},
				},
			},
			// swagger:route POST /organizations/{name}/teams organizations addTeamToOrganization
			//
			// Adds a team to an organization
			//
			// Adds a team to an organization.
			//
			//   Consumes:
			//   - application/json
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: TeamResponse
			gz.Method{
				Type:        "POST",
				Description: "Adds a team to an Organization",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, OrganizationTeamCreate))},
				},
			},
		},
	},
	// Route that returns information about an organization team
	gz.Route{
		Name:        "OrganizationTeamIndex",
		Description: "Route to get, update and delete a team of an organization",
		URI:         "/organizations/{name}/teams/{teamname}",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /organizations/{name}/teams/{teamname} organizations singleTeam
			//
			// Get a single team of an organization
			//
			// Return a team given its organization and team name.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: TeamResponse
			gz.Method{
				Type:        "GET",
				Description: "Get a team from an organization",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(NameHandler("name", true, OrganizationTeamIndex))},
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, OrganizationTeamIndex))},
				},
			},
			// swagger:route PATCH /organizations/{name}/teams/{teamname} organizations orgTeamUpdate
			//
			// Updates a team
			//
			// Updates a team of an organization
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: TeamResponse
			gz.Method{
				Type:        "PATCH",
				Description: "Updates a team",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, OrganizationTeamUpdate))},
				},
			},
			// swagger:route DELETE /organizations/{name}/teams/{teamname} organizations orgTeamDelete
			//
			// Removes a team
			//
			// Removes a team from an organization
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: TeamResponse
			gz.Method{
				Type:        "DELETE",
				Description: "Removes a team",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, OrganizationTeamRemove))},
				},
			},
		},
	},
	// Route to create an elastic search config
	gz.Route{
		Name:        "ElasticSearch",
		Description: "Route to create an ElasticSearch config",
		URI:         "/admin/search",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /admin/search search elasticSearchUpdate
			//
			// Get a list of the available ElasticSearch configurations.
			//
			// Zero or more ElasticSearch configurations may be specified. The
			// configuration marked as `primary` is the active ElasticSearch server.
			//
			//   Parameters:
			//   + name: Private-Token
			//     description: A personal access token.
			//     in: header
			//     required: true
			//     type: string
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: ElasticSearchConfigs
			gz.Method{
				Type:        "GET",
				Description: "Gets a list of the ElasticSearch configs",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(ListElasticSearchHandler)},
				},
			},

			// swagger:route POST /admin/search search elasticSearchUpdate
			//
			// Creates an ElasticSearch server configuration.
			//
			// Use this route to tell Fuel about a new ElasticSearch server.
			//
			//   Parameters:
			//   + name: Private-Token
			//     description: A personal access token.
			//     in: header
			//     required: true
			//     type: string
			//   + name: address
			//     description: URL address of an Elastic Search server.
			//     in: body
			//     required: true
			//     type: string
			//   + name: primary
			//     description: "true" to make this configuration the primary config.
			//     in: body
			//     required: false
			//     type: string
			//     default: false
			//   + name: username
			//     description: Username for ElasticSearch authentication
			//     in: body
			//     required: false
			//     type: string
			//   + name: password
			//     description: Password for ElasticSearch authentication
			//     in: body
			//     required: false
			//     type: string
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: ElasticSearchConfig
			gz.Method{
				Type:        "POST",
				Description: "Creates an ElasticSearch config",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(CreateElasticSearchHandler)},
				},
			},
		},
	},
	// Route to reconnect to the primary elastic search config
	gz.Route{
		Name:        "ElasticSearch",
		Description: "Route to reconnect to the primary elastic search config",
		URI:         "/admin/search/reconnect",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /admin/search/reconnect search elasticSearchUpdate
			//
			// Reconnects to the primary ElasticSearch server.
			//
			//   Parameters:
			//   + name: Private-Token
			//     description: A personal access token.
			//     in: header
			//     required: true
			//     type: string
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: AdminSearchResponse
			gz.Method{
				Type:        "GET",
				Description: "Reconnect to the primary ElasticSearch config",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(ReconnectElasticSearchHandler)},
				},
			},
		},
	},
	// Route to rebuild to the primary elastic search indices
	gz.Route{
		Name:        "ElasticSearch",
		Description: "Route to rebuild to the primary elastic search indices",
		URI:         "/admin/search/rebuild",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /admin/search/rebuild search elasticSearchUpdate
			//
			// Rebuilds the primary ElasticSearch indices.
			//
			// Rebuilding the indices may take several minutes. Use this route when
			// or if the ElasticSearch indices have become out of date.
			//
			//   Parameters:
			//   + name: Private-Token
			//     description: A personal access token.
			//     in: header
			//     required: true
			//     type: string
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: AdminSearchResponse
			gz.Method{
				Type:        "GET",
				Description: "Rebuild the primary ElasticSearch indices",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(RebuildElasticSearchHandler)},
				},
			},
		},
	},
	// Route to update to the primary elastic search indices
	gz.Route{
		Name:        "ElasticSearch",
		Description: "Route to update to the primary elastic search indices",
		URI:         "/admin/search/update",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /admin/search/update search elasticSearchUpdate
			//
			// Updates the primary ElasticSearch servers indices.
			//
			// This route will populate the primary ElasticSearch server with new
			// data contained in the Fuel database. This route may take several
			// minutes to complete.
			//
			//   Parameters:
			//   + name: Private-Token
			//     description: A personal access token.
			//     in: header
			//     required: true
			//     type: string
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: AdminSearchResponse
			gz.Method{
				Type:        "GET",
				Description: "Update the primary ElasticSearch indices",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(UpdateElasticSearchHandler)},
				},
			},
		},
	},
	// Route to manage an elastic search config
	gz.Route{
		Name:        "ElasticSearch",
		Description: "Route to manage an ElasticSearch config",
		URI:         "/admin/search/{config_id}",
		Headers:     gz.AuthHeadersOptional,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route DELETE /admin/search/{config_id} search elasticSearchUpdate
			//
			// Deletes an ElasticSearch server configuration.
			//
			// Use this route to remove and ElasticSearch configuration.
			//
			//   Parameters:
			//   + name: Private-Token
			//     description: A personal access token.
			//     in: header
			//     required: true
			//     type: string
			//   + name: config_id
			//     description: ID of the ElasticSearch configuration.
			//     in: path
			//     required: true
			//     type: integer
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: ElasticSearchConfig
			gz.Method{
				Type:        "DELETE",
				Description: "Deletes an ElasticSearch config",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(DeleteElasticSearchHandler)},
				},
			},
			// swagger:route PATCH /admin/search/{config_id} search elasticSearchUpdate
			//
			// Updates an ElasticSearch server configuration.
			//
			// Set the username, password, address, and primary status of an
			// ElasticSearch server configuration.
			//
			//   Parameters:
			//   + name: Private-Token
			//     description: A personal access token.
			//     in: header
			//     required: true
			//     type: string
			//   + name: config_id
			//     description: ID of the ElasticSearch configuration.
			//     in: path
			//     required: true
			//     type: integer
			//   + name: address
			//     description: URL address of an Elastic Search server.
			//     in: body
			//     required: true
			//     type: string
			//   + name: primary
			//     description: "true" to make this configuration the primary config.
			//     in: body
			//     required: false
			//     type: string
			//     default: false
			//   + name: username
			//     description: Username for ElasticSearch authentication
			//     in: body
			//     required: false
			//     type: string
			//   + name: password
			//     description: Password for ElasticSearch authentication
			//     in: body
			//     required: false
			//     type: string
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: ElasticSearchConfig
			gz.Method{
				Type:        "PATCH",
				Description: "Modify an ElasticSearch config",
				// Format handlers
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(ModifyElasticSearchHandler)},
				},
			},
		},
	},

	///////////////////
	// Model Reviews //
	///////////////////

	// Route for all model reviews
	gz.Route{
		Name:        "ModelReviews",
		Description: "Information about all model reviews",
		URI:         "/models/reviews",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /models/reviews reviews listModelReviews
			//
			// Get list of reviews for models.
			//
			// Get a list of reviews. reviews will be returned paginated,
			// with pages of 20 reviews by default. The user can request a
			// different page with query parameter 'page', and the page size
			// can be defined with query parameter 'per_page'.
			// The route supports the 'order' parameter, with values 'asc' and
			// 'desc' (default: desc).
			// It also supports the 'q' parameter to perform a fulltext search on reviews
			// name, description and tags.
			//
			//   Produces:
			//   - application/json
			//   - application/x-protobuf
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: jsonReviews
			gz.Method{
				Type:        "GET",
				Description: "Get all reviews for models",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(SearchHandler(ModelReviewList))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(SearchHandler(ModelReviewList))},
					gz.FormatHandler{Handler: gz.JSONResult(SearchHandler(ModelReviewList))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /models/reviews reviews createModelReview
			//
			// Create a new model and a new review.
			//
			gz.Method{
				Type:        "POST",
				Description: "Post a review and a new model",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(ModelReviewCreate)},
				},
			},
		},
	},

	gz.Route{
		Name:        "Review",
		Description: "Information about reviews for a model",
		URI:         "/{username}/models/{model}/reviews",
		Headers:     gz.AuthHeadersOptional,
		Methods: gz.Methods{
			// swagger:route GET /{username}/models/{model}/reviews reviews listUserModelReviews
			//
			// Get list of reviews for a model.
			//
			// Get a list of reviews. reviews will be returned paginated,
			// with pages of 20 reviews by default. The user can request a
			// different page with query parameter 'page', and the page size
			// can be defined with query parameter 'per_page'.
			// The route supports the 'order' parameter, with values 'asc' and
			// 'desc' (default: desc).
			// It also supports the 'q' parameter to perform a fulltext search on reviews
			// name, description and tags.
			//
			//   Produces:
			//   - application/json
			//   - application/x-protobuf
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: jsonReviews
			gz.Method{
				Type:        "GET",
				Description: "Get all reviews for a selected model",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(SearchHandler(UserModelReview))},
					gz.FormatHandler{Extension: ".proto", Handler: gz.ProtoResult(SearchHandler(UserModelReview))},
					gz.FormatHandler{Handler: gz.JSONResult(SearchHandler(UserModelReview))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /{username}/models/{model}/reviews reviews createUserModelReview
			//
			// Create a new review for an existing model.
			//
			gz.Method{
				Type:        "POST",
				Description: "Post a review for a model",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(ReviewCreate)},
				},
			},
		},
	},
} // routes
