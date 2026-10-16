package sdf

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// ValidationError is one problem found during SDF validation.
type ValidationError struct {
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}

// ValidationResult is the outcome of validating an SDF string.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// Validate performs lightweight structural checks on an SDF document:
//   - must parse as XML
//   - root must be <sdf>
//   - sdf must have a version attribute
//   - top-level must contain at least one of: world, model, actor, light
//   - joints with missing parent/child blocks get flagged
//
// This is intentionally NOT a full schema-conformance check against
// libsdformat — see design §13.10.
func Validate(input string) ValidationResult {
	res := ValidationResult{Valid: true}

	var root struct {
		XMLName xml.Name
		Version string `xml:"version,attr"`
		Inner   []struct {
			XMLName xml.Name
		} `xml:",any"`
	}
	if err := xml.Unmarshal([]byte(input), &root); err != nil {
		return ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Message: fmt.Sprintf("malformed XML: %v", err)}},
		}
	}

	if root.XMLName.Local != "sdf" {
		res.Valid = false
		res.Errors = append(res.Errors, ValidationError{
			Message: fmt.Sprintf("root element must be <sdf>, got <%s>", root.XMLName.Local),
		})
	}
	if root.Version == "" {
		res.Valid = false
		res.Errors = append(res.Errors, ValidationError{
			Message: "<sdf> must have a version attribute",
		})
	}

	if len(root.Inner) == 0 {
		res.Valid = false
		res.Errors = append(res.Errors, ValidationError{
			Message: "<sdf> must contain at least one child (world, model, actor, or light)",
		})
	} else {
		ok := false
		for _, c := range root.Inner {
			switch c.XMLName.Local {
			case "world", "model", "actor", "light":
				ok = true
			}
		}
		if !ok {
			res.Valid = false
			res.Errors = append(res.Errors, ValidationError{
				Message: "<sdf> must contain at least one of: world, model, actor, light",
			})
		}
	}

	if strings.Contains(input, "<joint") && !strings.Contains(input, "<parent") {
		res.Errors = append(res.Errors, ValidationError{
			Message: "joint is missing <parent> element",
		})
		res.Valid = false
	}
	if strings.Contains(input, "<joint") && !strings.Contains(input, "<child") {
		res.Errors = append(res.Errors, ValidationError{
			Message: "joint is missing <child> element",
		})
		res.Valid = false
	}

	return res
}
