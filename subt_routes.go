package main

import (
	"bitbucket.org/ignitionrobotics/ign-go"
)

var subTRoutes = ign.Routes{

	// REGISTRATIONS

	ign.Route{
		"Registrations",
		"Information about all SubT registrations",
		"/registrations",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Create a new subt registration",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(SubTRegistrationCreate)},
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
			ign.Method{
				"GET",
				"Get all subt registrations",
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(PaginationHandlerWithUser(RegistrationsList, true))},
					ign.FormatHandler{"", ign.JSONResult(PaginationHandlerWithUser(RegistrationsList, true))},
				},
			},
		},
	},

	ign.Route{
		"Single Registration",
		"Update a registration",
		"/registrations/{competition}/{name}",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"PATCH",
				"Resolves a subt registration",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, SubTRegistrationUpdate))},
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
			ign.Method{
				"DELETE",
				"Deletes a subt registration",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, SubTRegistrationDelete))},
				},
			},
		},
	},

	// PARTICIPANTS

	ign.Route{
		"Participants",
		"Information about all SubT participants",
		"/participants",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Get all subt participants",
				ign.FormatHandlers{
					ign.FormatHandler{".json", ign.JSONResult(PaginationHandlerWithUser(SubTParticipantsList, true))},
					ign.FormatHandler{"", ign.JSONResult(PaginationHandlerWithUser(SubTParticipantsList, true))},
				},
			},
		},
	},

	ign.Route{
		"Single Participant",
		"Update a participant",
		"/participants/{competition}/{name}",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"DELETE",
				"Delete a subt participant",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(NameHandler("name", true, SubTParticipantDelete))},
				},
			},
		},
	},
	ign.Route{
		"Participant Log Files",
		"SubT log files submissions from a participant",
		"/participants/{name}/logfiles",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Get a list of subt logfiles",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(PaginationHandlerWithUser(SubTLogFileList, true))},
				},
			},
		},
	},

	// LOG FILES

	ign.Route{
		"Log Files",
		"SubT log files submissions",
		"/logfiles",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"POST",
				"Create a new subt log file",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(SubTSubmitLogFile)},
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
			ign.Method{
				"GET",
				"Get all subt logfiles",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(PaginationHandlerWithUser(SubTLogFileList, true))},
				},
			},
		},
	},

	ign.Route{
		"Single Log File",
		"Single log files",
		"/logfiles/{id}",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Get a log file",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(SubTGetLogFile)},
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
			ign.Method{
				"PATCH",
				"Update a subt log file",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(SubTUpdateLogFile)},
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
			ign.Method{
				"DELETE",
				"Delete a log file",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(SubTDeleteLogFile)},
				},
			},
		},
	},

	ign.Route{
		"Download Single Log File",
		"Download Single log files",
		"/logfiles/{id}/file",
		ign.AuthHeadersRequired,
		ign.Methods{},
		ign.SecureMethods{
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
			ign.Method{
				"GET",
				"Get a log file",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(SubTLogFileDownload)},
				},
			},
		},
	},

	// LEADERBOARD

	ign.Route{
		"Leaderboard",
		"SubT leaderboard",
		"/leaderboard",
		ign.AuthHeadersRequired,
		ign.Methods{
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
			ign.Method{
				"GET",
				"Get the leaderboard",
				ign.FormatHandlers{
					ign.FormatHandler{"", ign.JSONResult(PaginationHandler(Leaderboard))},
				},
			},
		},
		ign.SecureMethods{},
	},
} // routes
