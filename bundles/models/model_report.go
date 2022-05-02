package models

import (
	"github.com/gazebo-web/fuel-server/bundles/generics"
)

// ModelReport contains information about a model's user report
type ModelReport struct {
	generics.Report

	// ModelID represents the model that was reported
	ModelID *uint `json:"model,omitempty"`
}
