package main

import (
	"gitlab.com/ignitionrobotics/web/ign-go"
)

/////////////////////////////////////////////////
/// Declare the routes. See also router.go
var routes = ign.Routes{

	////////////
	// Models //
	////////////

	// Route for all models
	ign.Route{
		"Models",
		"Information about all models",
		"/models",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get all models",
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONListResult("Models", SearchHandler(ModelList))},
					ign.FormatHandler{".proto", ign.ProtoResult(SearchHandler(ModelList))},
					ign.FormatHandler{"", ign.JSONListResult("Models", SearchHandler(ModelList))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Create a new model",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(ModelCreate)},
				},
			},
		},
	},

	// Route that returns a list of models from a team/user (ie. an 'owner')
	ign.Route{
		"OwnerModels",
		"Information about models belonging to an owner. The {username} URI option will limit the scope to the specified user/team. Otherwise all models are considered.",
		"/{username}/models",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get all models of the specified team/user",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONListResult("Models", SearchHandler(ModelList))},
					ign.FormatHandler{".proto", ign.ProtoResult(SearchHandler(ModelList))},
					ign.FormatHandler{"", ign.JSONListResult("Models", SearchHandler(ModelList))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that handles likes to a model from an owner
	ign.Route{
		"ModelLikes",
		"Handles the likes of a model.",
		"/{username}/models/{model}/likes",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Like a model",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("model", true, ModelOwnerLikeCreate)))},
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
			ign.Method{
				"DELETE",
				"Unlike a model",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("model", true, ModelOwnerLikeRemove)))},
				},
			},
		},
	},

	// Route that returns a list of models liked by a user.
	ign.Route{
		"ModelLikeList",
		"Models liked by a user.",
		"/{username}/likes/models",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get all models liked by the specified user",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONListResult("Models", SearchHandler(ModelLikeList))},
					ign.FormatHandler{"", ign.JSONListResult("Models", SearchHandler(ModelLikeList))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that returns the files tree of a single model based on owner, model name, and version
	ign.Route{
		"ModelOwnerVersionFileTree",
		"Route that returns the files tree of a single model.",
		"/{username}/models/{model}/{version}/files",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get file tree",
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameOwnerHandler("model", false, ModelOwnerVersionFileTree))},
					ign.FormatHandler{".proto", ign.ProtoResult(NameOwnerHandler("model", false, ModelOwnerVersionFileTree))},
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("model", false, ModelOwnerVersionFileTree))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that downloads an individual file from a model based on owner, model name, and version
	ign.Route{
		"ModelOwnerVersionIndividualFileDownload",
		"Download individual file from a model.",
		"/{username}/models/{model}/{version}/files/{path:.+}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"GET a file",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("model", false, ModelOwnerVersionIndividualFileDownload)))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that returns a model, by name, from a team/user
	ign.Route{
		"OwnerModelIndex",
		"Information about a model belonging to an owner.",
		"/{username}/models/{model}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get a model belonging to the specified team/user",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
					ign.FormatHandler{".proto", ign.ProtoResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
					ign.FormatHandler{".zip", ign.Handler(NoResult(NameOwnerHandler("model", false, ModelOwnerVersionZip)))},
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"PATCH",
				"Edit a model",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("model", true, ModelUpdate))},
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
			ign.Method{
				"DELETE",
				"Deletes a single model",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("model", true, ModelOwnerRemove)))},
				},
			},
		},
	},

	// Route that transfers a model
	ign.Route{
		"OwnerModelIndex",
		"Transfer a model to another owner.",
		"/{username}/models/{model}/transfer",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Transfer a model",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("model", true, ModelTransfer))},
				},
			},
		},
	},

	// Route that returns a model zip file from a team/user
	ign.Route{
		"OwnerModelVersion",
		"Download a versioned model zip file belonging to an owner.",
		"/{username}/models/{model}/{version}/{model}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get a model of specified version belonging to the specified team/user",
				// Format handlers
				// if empty file extension is given, it returns model's meta data
				// and {version} is then ignored
				ign.FormatHandlers{
					ign.FormatHandler{".zip", ign.Handler(NoResult(NameOwnerHandler("model", false, ModelOwnerVersionZip)))},
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("model", false, ModelOwnerIndex))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that clones a model
	ign.Route{
		"CloneModel",
		"Clone a model",
		"/{username}/models/{model}/clone",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Clones a model",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("model", false, ModelClone))},
				},
			},
		},
	},

	// Route that handles model reports
	ign.Route{
		"ReportModel",
		"Report a model",
		"/{username}/models/{model}/report",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"POST",
				"Reports a model",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("model", false, ReportModelCreate)))},
				},
			},
		},
		ign.SecureMethods{},
	},

	////////////
	// Worlds //
	////////////

	// Route for all worlds
	ign.Route{
		"Worlds",
		"Information about all worlds",
		"/worlds",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get all worlds",
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONListResult("Worlds", SearchHandler(WorldList))},
					ign.FormatHandler{".proto", ign.ProtoResult(SearchHandler(WorldList))},
					ign.FormatHandler{"", ign.JSONListResult("Worlds", SearchHandler(WorldList))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Create a new world",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(WorldCreate)},
				},
			},
		},
	},

	// Route that returns a list of worlds from a team/user (ie. an 'owner')
	ign.Route{
		"OwnerWorlds",
		"Information about worlds belonging to an owner. The {username} URI option will limit the scope to the specified user/team. Otherwise all worlds are considered.",
		"/{username}/worlds",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get all worlds of the specified team/user",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONListResult("Worlds", SearchHandler(WorldList))},
					ign.FormatHandler{".proto", ign.ProtoResult(SearchHandler(WorldList))},
					ign.FormatHandler{"", ign.JSONListResult("Worlds", SearchHandler(WorldList))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that handles likes to a world from an owner
	ign.Route{
		"WorldLikes",
		"Handles the likes of a world.",
		"/{username}/worlds/{world}/likes",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Like a world",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("world", true, WorldLikeCreate)))},
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
			ign.Method{
				"DELETE",
				"Unlike a world",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("world", true, WorldLikeRemove)))},
				},
			},
		},
	},

	// Route that returns a list of worlds liked by a user.
	ign.Route{
		"WorldLikeList",
		"Worlds liked by a user.",
		"/{username}/likes/worlds",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get all worlds liked by the specified user",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONListResult("Worlds", SearchHandler(WorldLikeList))},
					ign.FormatHandler{"", ign.JSONListResult("Worlds", SearchHandler(WorldLikeList))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that returns the files tree of a single world based on owner, name, and version
	ign.Route{
		"WorldFileTree",
		"Route that returns the files tree of a single world.",
		"/{username}/worlds/{world}/{version}/files",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get file tree",
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameOwnerHandler("world", false, WorldFileTree))},
					ign.FormatHandler{".proto", ign.ProtoResult(NameOwnerHandler("world", false, WorldFileTree))},
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("world", false, WorldFileTree))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that downloads an individual file from a world based on owner, name, and version
	ign.Route{
		"WorldIndividualFileDownload",
		"Download individual file from a world.",
		"/{username}/worlds/{world}/{version}/files/{path:.+}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"GET a file",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("world", false, WorldIndividualFileDownload)))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that returns a world, by name, from a team/user
	ign.Route{
		"WorldIndex",
		"Information about a world belonging to an owner.",
		"/{username}/worlds/{world}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get a world belonging to the specified team/user",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameOwnerHandler("world", false, WorldIndex))},
					ign.FormatHandler{".proto", ign.ProtoResult(NameOwnerHandler("world", false, WorldIndex))},
					ign.FormatHandler{".zip", ign.Handler(NoResult(NameOwnerHandler("world", false, WorldZip)))},
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("world", false, WorldIndex))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"PATCH",
				"Edit a world",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("world", true, WorldUpdate))},
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
			ign.Method{
				"DELETE",
				"Deletes a single world",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("world", true, WorldRemove)))},
				},
			},
		},
	},

	// Route that transfers a world
	ign.Route{
		"OwnerWorldTransfer",
		"Transfer a world to another owner.",
		"/{username}/worlds/{world}/transfer",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Transfer a world",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("world", true, WorldTransfer))},
				},
			},
		},
	},

	// Route that returns a world zip file from a team/user
	ign.Route{
		"WorldVersion",
		"Download a versioned world zip file belonging to an owner.",
		"/{username}/worlds/{world}/{version}/{world}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get a world of specified version belonging to the specified team/user",
				// Format handlers
				// if empty file extension is given, it returns world's meta data
				// and {version} is then ignored
				ign.FormatHandlers{
					ign.FormatHandler{".zip", ign.Handler(NoResult(NameOwnerHandler("world", false, WorldZip)))},
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("world", false, WorldIndex))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that clones a world
	ign.Route{
		"CloneWorld",
		"Clone a world",
		"/{username}/worlds/{world}/clone",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Clones a world",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("world", false, WorldClone))},
				},
			},
		},
	},

	// Route that handles world reports
	ign.Route{
		"ReportWorld",
		"Report a world",
		"/{username}/worlds/{world}/report",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"POST",
				"Reports a world",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("world", false, ReportWorldCreate)))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that returns the modelIncludes of a world.
	ign.Route{
		"WorldModelIncludes",
		"Route that returns the external models referenced by a world",
		"/{username}/worlds/{world}/{version}/{world}/modelrefs",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"World's ModelIncludes ",
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameOwnerHandler("world", false, WorldModelReferences))},
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("world", false, WorldModelReferences))},
				},
			},
		},
		ign.SecureMethods{},
	},

	/////////////////
	// Collections //
	/////////////////

	// Route for all Collections
	ign.Route{
		"Collection",
		"Information about all collections",
		"/collections",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get all collections",
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(SearchHandler(CollectionList))},
					ign.FormatHandler{"", ign.JSONResult(SearchHandler(CollectionList))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Create a new collection",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(CollectionCreate)},
				},
			},
		},
	},

	// Route that returns a list of collections from a team/user (ie. an 'owner')
	ign.Route{
		"OwnerCollections",
		"Information about worlds belonging to an owner. The {username} URI option " +
			"will limit the scope to the specified user/team. Otherwise all collections are considered.",
		"/{username}/collections",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get all collections of the specified team/user",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(SearchHandler(CollectionList))},
					ign.FormatHandler{"", ign.JSONResult(SearchHandler(CollectionList))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that returns a Collection, by name, from a team/user
	ign.Route{
		"CollectionIndex",
		"Information about a collection belonging to an owner.",
		"/{username}/collections/{collection}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get a collection belonging to the specified team/user",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameOwnerHandler("collection", false, CollectionIndex))},
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("collection", false, CollectionIndex))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"PATCH",
				"Edit a collection",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("collection", true, CollectionUpdate))},
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
			ign.Method{
				"DELETE",
				"Deletes a single collection",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("collection", true, CollectionRemove)))},
				},
			},
		},
	},

	ign.Route{
		"OwnerCollectionTransfer",
		"Transfer a collection to another owner.",
		"/{username}/collections/{collection}/transfer",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Transfer a collection",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("collection", true, CollectionTransfer))},
				},
			},
		},
	},
	// Route that clones a collection
	ign.Route{
		"CloneCollection",
		"Clone a collection",
		"/{username}/collections/{collection}/clone",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Clones a collection",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("collection", false, CollectionClone))},
				},
			},
		},
	},

	// Route that downloads an individual file from a collection.
	// It is used to download the collection logo and banner.
	ign.Route{
		"CollectionIndividualFileDownload",
		"Download individual file from a collection.",
		"/{username}/collections/{collection}/{version}/files/{path:.+}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"GET a file",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("collection", false, CollectionIndividualFileDownload)))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route to list, add and remove models from collections.
	ign.Route{
		"CollectionModels",
		"Information about models from a collection",
		"/{username}/collections/{collection}/models",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get the models associated to a collection",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONListResult("Models", NameOwnerHandler("collection", false, CollectionModelsList))},
					ign.FormatHandler{".proto", ign.ProtoResult(NameOwnerHandler("collection", false, CollectionModelsList))},
					ign.FormatHandler{"", ign.JSONListResult("Models", NameOwnerHandler("collection", false, CollectionModelsList))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Add a model to a collection",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("collection", true, CollectionModelAdd)))},
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
			ign.Method{
				"DELETE",
				"Removes a model from a collection",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("collection", true, CollectionModelRemove)))},
				},
			},
		},
	},

	// Route to list, add and remove worlds from collections.
	ign.Route{
		"CollectionWorlds",
		"Information about Worlds from a collection",
		"/{username}/collections/{collection}/worlds",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get the worlds associated to a collection",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONListResult("Worlds", NameOwnerHandler("collection", false, CollectionWorldsList))},
					ign.FormatHandler{".proto", ign.ProtoResult(NameOwnerHandler("collection", false, CollectionWorldsList))},
					ign.FormatHandler{"", ign.JSONListResult("Worlds", NameOwnerHandler("collection", false, CollectionWorldsList))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Add a world to a collection",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("collection", true, CollectionWorldAdd)))},
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
			ign.Method{
				"DELETE",
				"Removes a world from a collection",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.Handler(NoResult(NameOwnerHandler("collection", true, CollectionWorldRemove)))},
				},
			},
		},
	},

	// Route that returns the list of collections associated to a model
	ign.Route{
		"ModelCollections",
		"List of collections associated to a model.",
		"/{username}/models/{model}/collections",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"List of collections associated to a model",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameOwnerHandler("model", false, ModelCollections))},
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("model", false, ModelCollections))},
				},
			},
		},
		ign.SecureMethods{},
	},

	// Route that returns the list of collections associated to a world
	ign.Route{
		"WorldCollections",
		"List of collections associated to a world.",
		"/{username}/worlds/{world}/collections",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"List of collections associated to a world",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameOwnerHandler("world", false, WorldCollections))},
					ign.FormatHandler{"", ign.JSONResult(NameOwnerHandler("world", false, WorldCollections))},
				},
			},
		},
		ign.SecureMethods{},
	},

	///////////
	// Users //
	///////////

	// Route that returns login information for a given JWT
	ign.Route{
		"Login",
		"Login a user",
		"/login",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Login a user",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(Login)},
				},
			},
		},
	},

	// Route that returns information about all users
	ign.Route{
		"Users",
		"Route for all users",
		"/users",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Get all users information",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(PaginationHandler(UserList))},
					ign.FormatHandler{"", ign.JSONResult(PaginationHandler(UserList))},
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
			ign.Method{
				"POST",
				"Create a new user",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(UserCreate)},
				},
			},
		},
	},

	// Route that returns information about a user
	ign.Route{
		"UserIndex",
		"Access information about a single user.",
		"/users/{username}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get user information",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameHandler("username", false, UserIndex))},
					ign.FormatHandler{"", ign.JSONResult(NameHandler("username", false, UserIndex))},
				},
			},
		},

		ign.SecureMethods{
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
			ign.Method{
				"DELETE",
				"Remove a user",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("username", true, UserRemove))},
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
			ign.Method{
				"PATCH",
				"Update a user",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("username", true, UserUpdate))},
				},
			},
		},
	},

	// Routes to get and create access tokens.
	ign.Route{
		"AccessTokens",
		"Routes to get and create access tokens.",
		"/users/{username}/access-tokens",
		ign.AuthHeadersRequired,
		ign.Methods{},

		ign.SecureMethods{
			// swagger:route GET /users/{username}/access-tokens users getAccessToken
			//
			// Get the acccess tokens for a user.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			ign.Method{
				"GET",
				"Get a user's access tokens",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(PaginationHandlerWithUser(AccessTokenList, true))},
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
			ign.Method{
				"POST",
				"Create an access token",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("username", true, AccessTokenCreate))},
				},
			},
		},
	},

	// Routes to revoke access tokens
	ign.Route{
		"AccessTokens",
		"Route to revoke access tokens.",
		"/users/{username}/access-tokens/revoke",
		ign.AuthHeadersRequired,
		ign.Methods{},

		ign.SecureMethods{
			// swagger:route POST /users/{username}/access-tokens/revoke users revokeAccessToken
			//
			// Delete an acccess token that belongs to a user.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			ign.Method{
				"POST",
				"Delete a user's access token",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("username", true, AccessTokenDelete))},
				},
			},
		},
	},

	// Route that returns the details of a single user or organization
	ign.Route{
		"OwnerProfile",
		"Access the details of a single user OR organization.",
		"/profile/{username}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get profile information",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameHandler("username", false, OwnerProfile))},
					ign.FormatHandler{"", ign.JSONResult(NameHandler("username", false, OwnerProfile))},
				},
			},
		},
		ign.SecureMethods{},
	},

	//////////////
	// Licenses //
	//////////////

	// Route that returns information about all available licenses
	ign.Route{
		"Licenses",
		"Route for all licenses",
		"/licenses",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get all licenses",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(PaginationHandler(LicenseList))},
					ign.FormatHandler{"", ign.JSONResult(PaginationHandler(LicenseList))},
				},
			},
		},
		ign.SecureMethods{},
	},

	//////////////
	// Categories //
	//////////////

	// Categories route with slug
	// PATCH:
	ign.Route{
		Name:        "Categories",
		Description: "Routes for categories with slug",
		URI:         "/categories/{slug}",
		Headers:     ign.AuthHeadersOptional,
		Methods:     ign.Methods{},
		SecureMethods: ign.SecureMethods{
			ign.Method{
				Type:        "PATCH",
				Description: "Update a category",
				Handlers: ign.FormatHandlers{
					ign.FormatHandler{
						Extension: "",
						Handler:   ign.JSONResult(CategoryUpdate),
					},
				},
			},
			ign.Method{
				Type:        "DELETE",
				Description: "Delete a category",
				Handlers: ign.FormatHandlers{
					ign.FormatHandler{
						Extension: "",
						Handler:   ign.JSONResult(CategoryDelete),
					},
				},
			},
		},
	},

	// Categories route
	// GET: Get the list of categories
	// POST: Create a new category
	ign.Route{
		Name:        "Categories",
		Description: "Route for categories",
		URI:         "/categories",
		Headers:     ign.AuthHeadersOptional,
		Methods: ign.Methods{
			ign.Method{
				Type:        "GET",
				Description: "Get all categories",
				// Format handlers
				Handlers: ign.FormatHandlers{
					ign.FormatHandler{
						Extension: ".json",
						Handler:   ign.JSONResult(CategoryList),
					},
					ign.FormatHandler{
						Extension: "",
						Handler:   ign.JSONResult(CategoryList),
					},
				},
			},
		},
		SecureMethods: ign.SecureMethods{
			ign.Method{
				Type:        "POST",
				Description: "Create a new category",
				Handlers: ign.FormatHandlers{
					ign.FormatHandler{
						Extension: "",
						Handler:   ign.JSONResult(CategoryCreate),
					},
				},
			},
		},
	},

	///////////////////
	// Organizations //
	///////////////////

	// Route that returns information about all organizations
	ign.Route{
		"Organizations",
		"Route for all organizations",
		"/organizations",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get all organizations information",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(PaginationHandler(OrganizationList))},
					ign.FormatHandler{"", ign.JSONResult(PaginationHandler(OrganizationList))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Create a new organization",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(OrganizationCreate)},
				},
			},
		},
	},

	// Route that returns information about an organization
	ign.Route{
		"OrganizationIndex",
		"Access information about a single organization.",
		"/organizations/{name}",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get organization information",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameHandler("name", false, OrganizationIndex))},
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", false, OrganizationIndex))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"DELETE",
				"Remove an organization",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, OrganizationRemove))},
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
			ign.Method{
				"PATCH",
				"Edit an organization",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, OrganizationUpdate))},
				},
			},
		},
	},
	// Route that returns information about organization users
	ign.Route{
		"OrganizationUsers",
		"Base route to list of users of an Organization",
		"/organizations/{name}/users",
		ign.AuthHeadersOptional,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get the list of users of an organization",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(PaginationHandler(OrganizationUserList))},
					ign.FormatHandler{"", ign.JSONResult(PaginationHandler(OrganizationUserList))},
				},
			},
		},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Adds a user to an Organization",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, OrganizationUserCreate))},
				},
			},
		},
	},
	// Route that returns information about organization users
	ign.Route{
		"OrganizationUserUpdate",
		"Route to update and delete a member of an organization",
		"/organizations/{name}/users/{username}",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"DELETE",
				"Removes a user from an organization",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, OrganizationUserRemove))},
				},
			},
		},
	},

	// Route that returns information about organization teams
	ign.Route{
		"OrganizationTeams",
		"Base route to list of teams of an Organization",
		"/organizations/{name}/teams",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Get the list of teams of an organization",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(PaginationHandler(OrganizationTeamsList))},
					ign.FormatHandler{"", ign.JSONResult(PaginationHandler(OrganizationTeamsList))},
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
			ign.Method{
				"POST",
				"Adds a team to an Organization",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, OrganizationTeamCreate))},
				},
			},
		},
	},
	// Route that returns information about an organization team
	ign.Route{
		"OrganizationTeamIndex",
		"Route to get, update and delete a team of an organization",
		"/organizations/{name}/teams/{teamname}",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Get a team from an organization",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(NameHandler("name", true, OrganizationTeamIndex))},
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, OrganizationTeamIndex))},
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
			ign.Method{
				"PATCH",
				"Updates a team",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, OrganizationTeamUpdate))},
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
			ign.Method{
				"DELETE",
				"Removes a team",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, OrganizationTeamRemove))},
				},
			},
		},
	},
	// Route to create an elastic search config
	ign.Route{
		"ElasticSearch",
		"Route to create an ElasticSearch config",
		"/admin/search",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Gets a list of the ElasticSearch configs",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(ListElasticSearchHandler)},
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
			ign.Method{
				"POST",
				"Creates an ElasticSearch config",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(CreateElasticSearchHandler)},
				},
			},
		},
	},
	// Route to reconnect to the primary elastic search config
	ign.Route{
		"ElasticSearch",
		"Route to reconnect to the primary elastic search config",
		"/admin/search/reconnect",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Reconnect to the primary ElasticSearch config",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(ReconnectElasticSearchHandler)},
				},
			},
		},
	},
	// Route to rebuild to the primary elastic search indices
	ign.Route{
		"ElasticSearch",
		"Route to rebuild to the primary elastic search indices",
		"/admin/search/rebuild",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Rebuild the primary ElasticSearch indices",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(RebuildElasticSearchHandler)},
				},
			},
		},
	},
	// Route to update to the primary elastic search indices
	ign.Route{
		"ElasticSearch",
		"Route to update to the primary elastic search indices",
		"/admin/search/update",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Update the primary ElasticSearch indices",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(UpdateElasticSearchHandler)},
				},
			},
		},
	},
	// Route to manage an elastic search config
	ign.Route{
		"ElasticSearch",
		"Route to manage an ElasticSearch config",
		"/admin/search/{config_id}",
		ign.AuthHeadersOptional,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"DELETE",
				"Deletes an ElasticSearch config",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(DeleteElasticSearchHandler)},
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
			ign.Method{
				"PATCH",
				"Modify an ElasticSearch config",
				// Format handlers
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(ModifyElasticSearchHandler)},
				},
			},
		},
	},
} // routes
