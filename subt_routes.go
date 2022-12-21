package main

import "github.com/gazebo-web/gz-go/v7"

var subTRoutes = gz.Routes{

	// REGISTRATIONS

	gz.Route{
		"Registrations",
		"Information about all SubT registrations",
		"/registrations",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"POST",
				"Create a new subt registration",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(SubTRegistrationCreate)},
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
				"GET",
				"Get all subt registrations",
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(PaginationHandlerWithUser(RegistrationsList, true))},
					gz.FormatHandler{"", gz.JSONResult(PaginationHandlerWithUser(RegistrationsList, true))},
				},
			},
		},
	},

	gz.Route{
		"Single Registration",
		"Update a registration",
		"/registrations/{competition}/{name}",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"PATCH",
				"Resolves a subt registration",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, SubTRegistrationUpdate))},
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
				"DELETE",
				"Deletes a subt registration",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, SubTRegistrationDelete))},
				},
			},
		},
	},

	// PARTICIPANTS

	gz.Route{
		"Participants",
		"Information about all SubT participants",
		"/participants",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Get all subt participants",
				gz.FormatHandlers{
					gz.FormatHandler{".json", gz.JSONResult(PaginationHandlerWithUser(SubTParticipantsList, true))},
					gz.FormatHandler{"", gz.JSONResult(PaginationHandlerWithUser(SubTParticipantsList, true))},
				},
			},
		},
	},

	gz.Route{
		"Single Participant",
		"Update a participant",
		"/participants/{competition}/{name}",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"DELETE",
				"Delete a subt participant",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(NameHandler("name", true, SubTParticipantDelete))},
				},
			},
		},
	},
	gz.Route{
		"Participant Log Files",
		"SubT log files submissions from a participant",
		"/participants/{name}/logfiles",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Get a list of subt logfiles",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(PaginationHandlerWithUser(SubTLogFileList, true))},
				},
			},
		},
	},

	// LOG FILES

	gz.Route{
		"Log Files",
		"SubT log files submissions",
		"/logfiles",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"POST",
				"Create a new subt log file",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(SubTSubmitLogFile)},
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
				"GET",
				"Get all subt logfiles",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(PaginationHandlerWithUser(SubTLogFileList, true))},
				},
			},
		},
	},

	gz.Route{
		"Single Log File",
		"Single log files",
		"/logfiles/{id}",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Get a log file",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(SubTGetLogFile)},
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
				"PATCH",
				"Update a subt log file",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(SubTUpdateLogFile)},
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
				"DELETE",
				"Delete a log file",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(SubTDeleteLogFile)},
				},
			},
		},
	},

	gz.Route{
		"Download Single Log File",
		"Download Single log files",
		"/logfiles/{id}/file",
		gz.AuthHeadersRequired,
		gz.Methods{},
		gz.SecureMethods{
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
				"GET",
				"Get a log file",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(SubTLogFileDownload)},
				},
			},
		},
	},

	// LEADERBOARD

	gz.Route{
		"Leaderboard",
		"SubT leaderboard",
		"/leaderboard",
		gz.AuthHeadersRequired,
		gz.Methods{
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
				"GET",
				"Get the leaderboard",
				gz.FormatHandlers{
					gz.FormatHandler{"", gz.JSONResult(PaginationHandler(Leaderboard))},
				},
			},
		},
		gz.SecureMethods{},
	},
} // routes
