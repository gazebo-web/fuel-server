package main

import "github.com/gazebo-web/gz-go/v7"

var subTRoutes = gz.Routes{

	// REGISTRATIONS

	gz.Route{
		Name:        "Registrations",
		Description: "Information about all SubT registrations",
		URI:         "/registrations",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /subt/registrations subtRegistrations applySubtReg
			//
			// Apply a SubT registration
			//
			// Creates a new pending registration for SubT.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Registration
			gz.Method{
				Type:        "POST",
				Description: "Create a new subt registration",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(SubTRegistrationCreate)},
				},
			},
			// swagger:route GET /subt/registrations registrations listRegistrations
			//
			// Get list of Subt registrations.
			//
			// Get a list of registrations.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Registrations
			gz.Method{
				Type:        "GET",
				Description: "Get all subt registrations",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(PaginationHandlerWithUser(RegistrationsList, true))},
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandlerWithUser(RegistrationsList, true))},
				},
			},
		},
	},

	gz.Route{
		Name:        "Single Registration",
		Description: "Update a registration",
		URI:         "/registrations/{competition}/{name}",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route PATCH /subt/registrations/{competition}/{name} subtRegistrations resolveSubtReg
			//
			// Resolves a SubT registration
			//
			// Resolves a pending registration for SubT into Done or Rejected.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Registration
			gz.Method{
				Type:        "PATCH",
				Description: "Resolves a subt registration",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, SubTRegistrationUpdate))},
				},
			},
			// swagger:route DELETE /subt/registrations/{competition}/{name} subtRegistrations deleteSubtReg
			//
			// Deletes a SubT registration
			//
			// Deletes a pending registration for SubT.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Registration
			gz.Method{
				Type:        "DELETE",
				Description: "Deletes a subt registration",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, SubTRegistrationDelete))},
				},
			},
		},
	},

	// PARTICIPANTS

	gz.Route{
		Name:        "Participants",
		Description: "Information about all SubT participants",
		URI:         "/participants",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /subt/participants participants listParticipants
			//
			// Get list of Subt participants.
			//
			// Get a list of participants.
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
				Description: "Get all subt participants",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Extension: ".json", Handler: gz.JSONResult(PaginationHandlerWithUser(SubTParticipantsList, true))},
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandlerWithUser(SubTParticipantsList, true))},
				},
			},
		},
	},

	gz.Route{
		Name:        "Single Participant",
		Description: "Update a participant",
		URI:         "/participants/{competition}/{name}",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route DELETE /subt/participants/{competition}/{name} participants deleteSubtParticipants
			//
			// Delete a Subt participant.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: CompetitionParticipant
			gz.Method{
				Type:        "DELETE",
				Description: "Delete a subt participant",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(NameHandler("name", true, SubTParticipantDelete))},
				},
			},
		},
	},
	gz.Route{
		Name:        "Participant Log Files",
		Description: "SubT log files submissions from a participant",
		URI:         "/participants/{name}/logfiles",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /subt/participants/{name}/logfiles logfiles listPartLogfiles
			//
			// Get list of Subt log files.
			//
			// Get a list of log files.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: LogFiles
			gz.Method{
				Type:        "GET",
				Description: "Get a list of subt logfiles",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandlerWithUser(SubTLogFileList, true))},
				},
			},
		},
	},

	// LOG FILES

	gz.Route{
		Name:        "Log Files",
		Description: "SubT log files submissions",
		URI:         "/logfiles",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route POST /subt/logfiles logfiles submitLog
			//
			// Submit a SubT log file
			//
			// Creates a new log file submission
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
			//     200: LogFile
			gz.Method{
				Type:        "POST",
				Description: "Create a new subt log file",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(SubTSubmitLogFile)},
				},
			},
			// swagger:route GET /subt/logfiles logfiles listLogfiles
			//
			// Get list of Subt log files.
			//
			// Get a list of log files.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: LogFiles
			gz.Method{
				Type:        "GET",
				Description: "Get all subt logfiles",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandlerWithUser(SubTLogFileList, true))},
				},
			},
		},
	},

	gz.Route{
		Name:        "Single Log File",
		Description: "Single log files",
		URI:         "/logfiles/{id}",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /subt/logfiles/{id} logfiles getLogFile
			//
			// Update a log file
			//
			// Updates a log file submission (eg. for scoring)
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: LogFile
			gz.Method{
				Type:        "GET",
				Description: "Get a log file",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(SubTGetLogFile)},
				},
			},
			// swagger:route PATCH /subt/logfiles/{id} logfiles updateLogFile
			//
			// Update a log file
			//
			// Updates a log file submission (eg. for scoring)
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: LogFile
			gz.Method{
				Type:        "PATCH",
				Description: "Update a subt log file",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(SubTUpdateLogFile)},
				},
			},
			// swagger:route DELETE /subt/logfiles/{id} logfiles deleteLogFile
			//
			// Deletes a log file
			//
			// Deletes a log file submission
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: LogFile
			gz.Method{
				Type:        "DELETE",
				Description: "Delete a log file",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(SubTDeleteLogFile)},
				},
			},
		},
	},

	gz.Route{
		Name:        "Download Single Log File",
		Description: "Download Single log files",
		URI:         "/logfiles/{id}/file",
		Headers:     gz.AuthHeadersRequired,
		Methods:     gz.Methods{},
		SecureMethods: gz.SecureMethods{
			// swagger:route GET /subt/logfiles/{id}/file logfiles downloadLogFile
			//
			// Downloads a log file
			//
			// Downloads a log file
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: File
			gz.Method{
				Type:        "GET",
				Description: "Get a log file",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(SubTLogFileDownload)},
				},
			},
		},
	},

	// LEADERBOARD

	gz.Route{
		Name:        "Leaderboard",
		Description: "SubT leaderboard",
		URI:         "/leaderboard",
		Headers:     gz.AuthHeadersRequired,
		Methods: gz.Methods{
			// swagger:route GET /subt/leaderboard leaderboard listLeaderboard
			//
			// Get the Subt leaderboard.
			//
			// Get the Subt leaderboard.
			//
			//   Produces:
			//   - application/json
			//
			//   Schemes: https
			//
			//   Responses:
			//     default: fuelError
			//     200: Leaderboard
			gz.Method{
				Type:        "GET",
				Description: "Get the leaderboard",
				Handlers: gz.FormatHandlers{
					gz.FormatHandler{Handler: gz.JSONResult(PaginationHandler(Leaderboard))},
				},
			},
		},
		SecureMethods: gz.SecureMethods{},
	},
} // routes
