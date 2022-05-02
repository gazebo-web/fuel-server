package worlds

import "github.com/gazebo-web/fuel-server/bundles/generics"

// WorldReport contains information about a world's user report.
type WorldReport struct {
	generics.Report

	// WorldID represents the world that was reported
	WorldID *uint `json:"world,omitempty"`
}
