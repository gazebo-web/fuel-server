package worlds

import "bitbucket.org/ignitionrobotics/ign-fuelserver/bundles/generics"

// WorldReport contains information about a world's user report.
type WorldReport struct {
	generics.Report

	// WorldID represents the world that was reported
	WorldID *uint `json:"world,omitempty"`
}
