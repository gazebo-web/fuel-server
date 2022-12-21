package main

import (
	"github.com/gazebo-web/gz-go/v7"
)

// ///////////////////////////////////////////////
// / Declare the routes. See also router.go
var routes = gz.Routes{

	////////////
	// Models //
	////////////

	// Route for all models
	gz.Route{
		"Models",
		"Information about all models",
		"/models",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all models",
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONListResult("Models", SearchHandler(ModelList))},
					gz.FormatHandler{".proto", gz.ProtoResult(SearchHandler(ModelList))},
					gz.FormatHandler{"", gz.JSONListResult("Models", SearchHandler(ModelList))},
				},
			},
		},
		gz.SecureMethods{
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
				"POST",
				"Create a new model",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(ModelCreate)},
				},
			},
		},
	},

	// Route that returns a list of models from a team/user (ie. an 'owner')
	gz.Route{
		"OwnerModels",
		"Information about models belonging to an owner. The {username} URI option will limit the scope to the specified user/team. Otherwise all models are considered.",
		"/{username}/models",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all models of the specified team/user",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONListResult("Models", SearchHandler(ModelList))},
					gz.FormatHandler{".proto", gz.ProtoResult(SearchHandler(ModelList))},
					gz.FormatHandler{"", gz.JSONListResult("Models", SearchHandler(ModelList))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that handles likes to a model from an owner
	gz.Route{
		"ModelLikes",
		"Handles the likes of a model.",
		"/{username}/models/{model}/likes",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"POST",
				"Like a model",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("model", true, ModelOwnerLikeCreate)))},
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
				"DELETE",
				"Unlike a model",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("model", true, ModelOwnerLikeRemove)))},
				},
			},
		},
	},

	// Route that returns a list of models liked by a user.
	gz.Route{
		"ModelLikeList",
		"Models liked by a user.",
		"/{username}/likes/models",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all models liked by the specified user",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONListResult("Models", SearchHandler(ModelLikeList))},
					gz.FormatHandler{"", gz.JSONListResult("Models", SearchHandler(ModelLikeList))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that returns the files tree of a single model based on owner, model name, and version
	gz.Route{
		"ModelOwnerVersionFileTree",
		"Route that returns the files tree of a single model.",
		"/{username}/models/{model}/{version}/files",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get file tree",
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameOwnerHandler("model", false, ModelOwnerVersionFileTree))},
					gz.FormatHandler{".proto", gz.ProtoResult(NameOwnerHandler("model", false, ModelOwnerVersionFileTree))},
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("model", false, ModelOwnerVersionFileTree))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that downloads an individual file from a model based on owner, model name, and version
	gz.Route{
		"ModelOwnerVersionIndividualFileDownload",
		"Download individual file from a model.",
		"/{username}/models/{model}/{version}/files/{path:.+}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"GET a file",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("model", false, ModelOwnerVersionIndividualFileDownload)))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that returns a model, by name, from a team/user
	gz.Route{
		"OwnerModelIndex",
		"Information about a model belonging to an owner.",
		"/{username}/models/{model}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get a model belonging to the specified team/user",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
					gz.FormatHandler{".proto", gz.ProtoResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
					gz.FormatHandler{".zip", gz.Handler(NoResult(NameOwnerHandler("model", false, ModelOwnerVersionZip)))},
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
				},
			},
		},
		gz.SecureMethods{
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
				"PATCH",
				"Edit a model",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("model", true, ModelUpdate))},
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
				"DELETE",
				"Deletes a single model",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("model", true, ModelOwnerRemove)))},
				},
			},
		},
	},

	// Route that transfers a model
	gz.Route{
		"OwnerModelIndex",
		"Transfer a model to another owner.",
		"/{username}/models/{model}/transfer",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"POST",
				"Transfer a model",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("model", true, ModelTransfer))},
				},
			},
		},
	},

	// Route that returns a model zip file from a team/user
	gz.Route{
		"OwnerModelVersion",
		"Download a versioned model zip file belonging to an owner.",
		"/{username}/models/{model}/{version}/{model}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get a model of specified version belonging to the specified team/user",
				// Format handlers
				// if empty file extension is given, it returns model's meta data
				// and {version} is then ignored
				gz.FormatHandlers{
					gz.FormatHandler{".zip", gz.Handler(NoResult(NameOwnerHandler("model", false, ModelOwnerVersionZip)))},
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that clones a model
	gz.Route{
		"CloneModel",
		"Clone a model",
		"/{username}/models/{model}/clone",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"POST",
				"Clones a model",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("model", false, ModelClone))},
				},
			},
		},
	},

	// Route that handles model reports
	gz.Route{
		"ReportModel",
		"Report a model",
		"/{username}/models/{model}/report",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"POST",
				"Reports a model",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("model", false, ReportModelCreate)))},
				},
			},
		},
		gz.SecureMethods{},
	},

	////////////
	// Worlds //
	////////////

	// Route for all worlds
	gz.Route{
		"Worlds",
		"Information about all worlds",
		"/worlds",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all worlds",
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONListResult("Worlds", SearchHandler(WorldList))},
					gz.FormatHandler{".proto", gz.ProtoResult(SearchHandler(WorldList))},
					gz.FormatHandler{"", gz.JSONListResult("Worlds", SearchHandler(WorldList))},
				},
			},
		},
		gz.SecureMethods{
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
				"POST",
				"Create a new world",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(WorldCreate)},
				},
			},
		},
	},

	// Route that returns a list of worlds from a team/user (ie. an 'owner')
	gz.Route{
		"OwnerWorlds",
		"Information about worlds belonging to an owner. The {username} URI option will limit the scope to the specified user/team. Otherwise all worlds are considered.",
		"/{username}/worlds",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all worlds of the specified team/user",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONListResult("Worlds", SearchHandler(WorldList))},
					gz.FormatHandler{".proto", gz.ProtoResult(SearchHandler(WorldList))},
					gz.FormatHandler{"", gz.JSONListResult("Worlds", SearchHandler(WorldList))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that handles likes to a world from an owner
	gz.Route{
		"WorldLikes",
		"Handles the likes of a world.",
		"/{username}/worlds/{world}/likes",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"POST",
				"Like a world",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("world", true, WorldLikeCreate)))},
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
				"DELETE",
				"Unlike a world",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("world", true, WorldLikeRemove)))},
				},
			},
		},
	},

	// Route that returns a list of worlds liked by a user.
	gz.Route{
		"WorldLikeList",
		"Worlds liked by a user.",
		"/{username}/likes/worlds",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all worlds liked by the specified user",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONListResult("Worlds", SearchHandler(WorldLikeList))},
					gz.FormatHandler{"", gz.JSONListResult("Worlds", SearchHandler(WorldLikeList))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that returns the files tree of a single world based on owner, name, and version
	gz.Route{
		"WorldFileTree",
		"Route that returns the files tree of a single world.",
		"/{username}/worlds/{world}/{version}/files",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get file tree",
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameOwnerHandler("world", false, WorldFileTree))},
					gz.FormatHandler{".proto", gz.ProtoResult(NameOwnerHandler("world", false, WorldFileTree))},
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("world", false, WorldFileTree))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that downloads an individual file from a world based on owner, name, and version
	gz.Route{
		"WorldIndividualFileDownload",
		"Download individual file from a world.",
		"/{username}/worlds/{world}/{version}/files/{path:.+}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"GET a file",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("world", false, WorldIndividualFileDownload)))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that returns a world, by name, from a team/user
	gz.Route{
		"WorldIndex",
		"Information about a world belonging to an owner.",
		"/{username}/worlds/{world}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get a world belonging to the specified team/user",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameOwnerHandler("world", false, WorldIndex))},
					gz.FormatHandler{".proto", gz.ProtoResult(NameOwnerHandler("world", false, WorldIndex))},
					gz.FormatHandler{".zip", gz.Handler(NoResult(NameOwnerHandler("world", false, WorldZip)))},
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("world", false, WorldIndex))},
				},
			},
		},
		gz.SecureMethods{
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
				"PATCH",
				"Edit a world",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("world", true, WorldUpdate))},
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
				"DELETE",
				"Deletes a single world",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("world", true, WorldRemove)))},
				},
			},
		},
	},

	// Route that transfers a world
	gz.Route{
		"OwnerWorldTransfer",
		"Transfer a world to another owner.",
		"/{username}/worlds/{world}/transfer",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"POST",
				"Transfer a world",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("world", true, WorldTransfer))},
				},
			},
		},
	},

	// Route that returns a world zip file from a team/user
	gz.Route{
		"WorldVersion",
		"Download a versioned world zip file belonging to an owner.",
		"/{username}/worlds/{world}/{version}/{world}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get a world of specified version belonging to the specified team/user",
				// Format handlers
				// if empty file extension is given, it returns world's meta data
				// and {version} is then ignored
				gz.FormatHandlers{
					gz.FormatHandler{".zip", gz.Handler(NoResult(NameOwnerHandler("world", false, WorldZip)))},
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("world", false, WorldIndex))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that clones a world
	gz.Route{
		"CloneWorld",
		"Clone a world",
		"/{username}/worlds/{world}/clone",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"POST",
				"Clones a world",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("world", false, WorldClone))},
				},
			},
		},
	},

	// Route that handles world reports
	gz.Route{
		"ReportWorld",
		"Report a world",
		"/{username}/worlds/{world}/report",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"POST",
				"Reports a world",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("world", false, ReportWorldCreate)))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that returns the modelIncludes of a world.
	gz.Route{
		"WorldModelIncludes",
		"Route that returns the external models referenced by a world",
		"/{username}/worlds/{world}/{version}/{world}/modelrefs",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"World's ModelIncludes ",
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameOwnerHandler("world", false, WorldModelReferences))},
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("world", false, WorldModelReferences))},
				},
			},
		},
		gz.SecureMethods{},
	},

	/////////////////
	// Collections //
	/////////////////

	// Route for all Collections
	gz.Route{
		"Collection",
		"Information about all collections",
		"/collections",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all collections",
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(SearchHandler(CollectionList))},
					gz.FormatHandler{"", gz.JSONResult(SearchHandler(CollectionList))},
				},
			},
		},
		gz.SecureMethods{
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
				"POST",
				"Create a new collection",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(CollectionCreate)},
				},
			},
		},
	},

	// Route that returns a list of collections from a team/user (ie. an 'owner')
	gz.Route{
		"OwnerCollections",
		"Information about worlds belonging to an owner. The {username} URI option " +
			"will limit the scope to the specified user/team. Otherwise all collections are considered.",
		"/{username}/collections",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all collections of the specified team/user",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(SearchHandler(CollectionList))},
					gz.FormatHandler{"", gz.JSONResult(SearchHandler(CollectionList))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that returns a Collection, by name, from a team/user
	gz.Route{
		"CollectionIndex",
		"Information about a collection belonging to an owner.",
		"/{username}/collections/{collection}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get a collection belonging to the specified team/user",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameOwnerHandler("collection", false, CollectionIndex))},
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("collection", false, CollectionIndex))},
				},
			},
		},
		gz.SecureMethods{
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
				"PATCH",
				"Edit a collection",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("collection", true, CollectionUpdate))},
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
				"DELETE",
				"Deletes a single collection",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("collection", true, CollectionRemove)))},
				},
			},
		},
	},

	gz.Route{
		"OwnerCollectionTransfer",
		"Transfer a collection to another owner.",
		"/{username}/collections/{collection}/transfer",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"POST",
				"Transfer a collection",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("collection", true, CollectionTransfer))},
				},
			},
		},
	},
	// Route that clones a collection
	gz.Route{
		"CloneCollection",
		"Clone a collection",
		"/{username}/collections/{collection}/clone",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"POST",
				"Clones a collection",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("collection", false, CollectionClone))},
				},
			},
		},
	},

	// Route that downloads an individual file from a collection.
	// It is used to download the collection logo and banner.
	gz.Route{
		"CollectionIndividualFileDownload",
		"Download individual file from a collection.",
		"/{username}/collections/{collection}/{version}/files/{path:.+}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"GET a file",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("collection", false, CollectionIndividualFileDownload)))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route to list, add and remove models from collections.
	gz.Route{
		"CollectionModels",
		"Information about models from a collection",
		"/{username}/collections/{collection}/models",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get the models associated to a collection",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONListResult("Models", NameOwnerHandler("collection", false, CollectionModelsList))},
					gz.FormatHandler{".proto", gz.ProtoResult(NameOwnerHandler("collection", false, CollectionModelsList))},
					gz.FormatHandler{"", gz.JSONListResult("Models", NameOwnerHandler("collection", false, CollectionModelsList))},
				},
			},
		},
		gz.SecureMethods{
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
				"POST",
				"Add a model to a collection",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("collection", true, CollectionModelAdd)))},
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
				"DELETE",
				"Removes a model from a collection",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("collection", true, CollectionModelRemove)))},
				},
			},
		},
	},

	// Route to list, add and remove worlds from collections.
	gz.Route{
		"CollectionWorlds",
		"Information about Worlds from a collection",
		"/{username}/collections/{collection}/worlds",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get the worlds associated to a collection",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONListResult("Worlds", NameOwnerHandler("collection", false, CollectionWorldsList))},
					gz.FormatHandler{".proto", gz.ProtoResult(NameOwnerHandler("collection", false, CollectionWorldsList))},
					gz.FormatHandler{"", gz.JSONListResult("Worlds", NameOwnerHandler("collection", false, CollectionWorldsList))},
				},
			},
		},
		gz.SecureMethods{
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
				"POST",
				"Add a world to a collection",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("collection", true, CollectionWorldAdd)))},
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
				"DELETE",
				"Removes a world from a collection",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.Handler(NoResult(NameOwnerHandler("collection", true, CollectionWorldRemove)))},
				},
			},
		},
	},

	// Route that returns the list of collections associated to a model
	gz.Route{
		"ModelCollections",
		"List of collections associated to a model.",
		"/{username}/models/{model}/collections",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"List of collections associated to a model",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameOwnerHandler("model", false, ModelCollections))},
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("model", false, ModelCollections))},
				},
			},
		},
		gz.SecureMethods{},
	},

	// Route that returns the list of collections associated to a world
	gz.Route{
		"WorldCollections",
		"List of collections associated to a world.",
		"/{username}/worlds/{world}/collections",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"List of collections associated to a world",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameOwnerHandler("world", false, WorldCollections))},
					gz.FormatHandler{"", gz.JSONResult(NameOwnerHandler("world", false, WorldCollections))},
				},
			},
		},
		gz.SecureMethods{},
	},

	///////////
	// Users //
	///////////

	// Route that returns login information for a given JWT
	gz.Route{
		"Login",
		"Login a user",
		"/login",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Login a user",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(Login)},
				},
			},
		},
	},

	// Route that returns information about all users
	gz.Route{
		"Users",
		"Route for all users",
		"/users",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Get all users information",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(PaginationHandler(UserList))},
					gz.FormatHandler{"", gz.JSONResult(PaginationHandler(UserList))},
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
				"POST",
				"Create a new user",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(UserCreate)},
				},
			},
		},
	},

	// Route that returns information about a user
	gz.Route{
		"UserIndex",
		"Access information about a single user.",
		"/users/{username}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get user information",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameHandler("username", false, UserIndex))},
					gz.FormatHandler{"", gz.JSONResult(NameHandler("username", false, UserIndex))},
				},
			},
		},

		gz.SecureMethods{
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
				"DELETE",
				"Remove a user",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("username", true, UserRemove))},
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
				"PATCH",
				"Update a user",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("username", true, UserUpdate))},
				},
			},
		},
	},

	// Routes to get and create access tokens.
	gz.Route{
		"AccessTokens",
		"Routes to get and create access tokens.",
		"/users/{username}/access-tokens",
		gz.AuthHeadersRequired,
		gz.Methods{},

		gz.SecureMethods{
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
				"GET",
				"Get a user's access tokens",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(PaginationHandlerWithUser(AccessTokenList, true))},
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
				"POST",
				"Create an access token",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("username", true, AccessTokenCreate))},
				},
			},
		},
	},

	// Routes to revoke access tokens
	gz.Route{
		"AccessTokens",
		"Route to revoke access tokens.",
		"/users/{username}/access-tokens/revoke",
		gz.AuthHeadersRequired,
		gz.Methods{},

		gz.SecureMethods{
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
				"POST",
				"Delete a user's access token",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("username", true, AccessTokenDelete))},
				},
			},
		},
	},

	// Route that returns the details of a single user or organization
	gz.Route{
		"OwnerProfile",
		"Access the details of a single user OR organization.",
		"/profile/{username}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get profile information",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameHandler("username", false, OwnerProfile))},
					gz.FormatHandler{"", gz.JSONResult(NameHandler("username", false, OwnerProfile))},
				},
			},
		},
		gz.SecureMethods{},
	},

	//////////////
	// Licenses //
	//////////////

	// Route that returns information about all available licenses
	gz.Route{
		"Licenses",
		"Route for all licenses",
		"/licenses",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all licenses",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(PaginationHandler(LicenseList))},
					gz.FormatHandler{"", gz.JSONResult(PaginationHandler(LicenseList))},
				},
			},
		},
		gz.SecureMethods{},
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
		"Organizations",
		"Route for all organizations",
		"/organizations",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all organizations information",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(PaginationHandler(OrganizationList))},
					gz.FormatHandler{"", gz.JSONResult(PaginationHandler(OrganizationList))},
				},
			},
		},
		gz.SecureMethods{
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
				"POST",
				"Create a new organization",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(OrganizationCreate)},
				},
			},
		},
	},

	// Route that returns information about an organization
	gz.Route{
		"OrganizationIndex",
		"Access information about a single organization.",
		"/organizations/{name}",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get organization information",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameHandler("name", false, OrganizationIndex))},
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", false, OrganizationIndex))},
				},
			},
		},
		gz.SecureMethods{
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
				"DELETE",
				"Remove an organization",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, OrganizationRemove))},
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
				"PATCH",
				"Edit an organization",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, OrganizationUpdate))},
				},
			},
		},
	},
	// Route that returns information about organization users
	gz.Route{
		"OrganizationUsers",
		"Base route to list of users of an Organization",
		"/organizations/{name}/users",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get the list of users of an organization",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(PaginationHandler(OrganizationUserList))},
					gz.FormatHandler{"", gz.JSONResult(PaginationHandler(OrganizationUserList))},
				},
			},
		},
		gz.SecureMethods{
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
				"POST",
				"Adds a user to an Organization",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, OrganizationUserCreate))},
				},
			},
		},
	},
	// Route that returns information about organization users
	gz.Route{
		"OrganizationUserUpdate",
		"Route to update and delete a member of an organization",
		"/organizations/{name}/users/{username}",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"DELETE",
				"Removes a user from an organization",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, OrganizationUserRemove))},
				},
			},
		},
	},

	// Route that returns information about organization teams
	gz.Route{
		"OrganizationTeams",
		"Base route to list of teams of an Organization",
		"/organizations/{name}/teams",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Get the list of teams of an organization",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(PaginationHandler(OrganizationTeamsList))},
					gz.FormatHandler{"", gz.JSONResult(PaginationHandler(OrganizationTeamsList))},
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
				"POST",
				"Adds a team to an Organization",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, OrganizationTeamCreate))},
				},
			},
		},
	},
	// Route that returns information about an organization team
	gz.Route{
		"OrganizationTeamIndex",
		"Route to get, update and delete a team of an organization",
		"/organizations/{name}/teams/{teamname}",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Get a team from an organization",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(NameHandler("name", true, OrganizationTeamIndex))},
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, OrganizationTeamIndex))},
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
				"PATCH",
				"Updates a team",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, OrganizationTeamUpdate))},
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
				"DELETE",
				"Removes a team",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, OrganizationTeamRemove))},
				},
			},
		},
	},
	// Route to create an elastic search config
	gz.Route{
		"ElasticSearch",
		"Route to create an ElasticSearch config",
		"/admin/search",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Gets a list of the ElasticSearch configs",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(ListElasticSearchHandler)},
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
				"POST",
				"Creates an ElasticSearch config",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(CreateElasticSearchHandler)},
				},
			},
		},
	},
	// Route to reconnect to the primary elastic search config
	gz.Route{
		"ElasticSearch",
		"Route to reconnect to the primary elastic search config",
		"/admin/search/reconnect",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Reconnect to the primary ElasticSearch config",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(ReconnectElasticSearchHandler)},
				},
			},
		},
	},
	// Route to rebuild to the primary elastic search indices
	gz.Route{
		"ElasticSearch",
		"Route to rebuild to the primary elastic search indices",
		"/admin/search/rebuild",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Rebuild the primary ElasticSearch indices",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(RebuildElasticSearchHandler)},
				},
			},
		},
	},
	// Route to update to the primary elastic search indices
	gz.Route{
		"ElasticSearch",
		"Route to update to the primary elastic search indices",
		"/admin/search/update",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Update the primary ElasticSearch indices",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(UpdateElasticSearchHandler)},
				},
			},
		},
	},
	// Route to manage an elastic search config
	gz.Route{
		"ElasticSearch",
		"Route to manage an ElasticSearch config",
		"/admin/search/{config_id}",
		gz.AuthHeadersOptional,
		gz.Methods{},
		gz.SecureMethods{
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
				"DELETE",
				"Deletes an ElasticSearch config",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(DeleteElasticSearchHandler)},
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
				"PATCH",
				"Modify an ElasticSearch config",
				// Format handlers
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(ModifyElasticSearchHandler)},
				},
			},
		},
	},

	///////////////////
	// Model Reviews //
	///////////////////

	// Route for all model reviews
	gz.Route{
		"ModelReviews",
		"Information about all model reviews",
		"/models/reviews",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all reviews for models",
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(SearchHandler(ModelReviewList))},
					gz.FormatHandler{".proto", gz.ProtoResult(SearchHandler(ModelReviewList))},
					gz.FormatHandler{"", gz.JSONResult(SearchHandler(ModelReviewList))},
				},
			},
		},
		gz.SecureMethods{
			// swagger:route POST /models/reviews reviews createModelReview
			//
			// Create a new model and a new review.
			//
			gz.Method{
				"POST",
				"Post a review and a new model",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(ModelReviewCreate)},
				},
			},
		},
	},

	gz.Route{
		"Review",
		"Information about reviews for a model",
		"/{username}/models/{model}/reviews",
		gz.AuthHeadersOptional,
		gz.Methods{
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
				"GET",
				"Get all reviews for a selected model",
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(SearchHandler(UserModelReview))},
					gz.FormatHandler{".proto", gz.ProtoResult(SearchHandler(UserModelReview))},
					gz.FormatHandler{"", gz.JSONResult(SearchHandler(UserModelReview))},
				},
			},
		},
		gz.SecureMethods{
			// swagger:route POST /{username}/models/{model}/reviews reviews createUserModelReview
			//
			// Create a new review for an existing model.
			//
			gz.Method{
				"POST",
				"Post a review for a model",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(ReviewCreate)},
				},
			},
		},
	},
} // routes
