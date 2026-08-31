// Package nodecatalog defines the product-only Node Catalog and its small Gum
// Config Schema. It does not extend the historical workflow/v1 definitions.
package nodecatalog

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// FieldType is one of the first-version Gum Config Schema value kinds.
type FieldType string

// Gum Config Schema v1 field types.
const (
	// FieldString stores plain text.
	FieldString FieldType = "string"
	// FieldMarkdown stores markdown text.
	FieldMarkdown FieldType = "markdown"
	// FieldInteger stores a whole number.
	FieldInteger FieldType = "integer"
	// FieldNumber stores a finite number.
	FieldNumber FieldType = "number"
	// FieldBoolean stores true or false.
	FieldBoolean FieldType = "boolean"
	// FieldEnum stores one declared string value.
	FieldEnum FieldType = "enum"
)

// PresentationHint changes how a field is shown without changing validation.
type PresentationHint struct {
	Label  string `json:"label,omitempty"`
	Help   string `json:"help,omitempty"`
	Editor string `json:"editor,omitempty"`
}

// ConfigField combines a semantic field contract with optional presentation.
type ConfigField struct {
	Name         string           `json:"name"`
	Type         FieldType        `json:"type"`
	Required     bool             `json:"required"`
	Default      any              `json:"default,omitempty"`
	HasDefault   bool             `json:"hasDefault"`
	Min          *float64         `json:"min,omitempty"`
	Max          *float64         `json:"max,omitempty"`
	Values       []string         `json:"values,omitempty"`
	Sensitive    bool             `json:"sensitive"`
	Presentation PresentationHint `json:"presentation"`
}

// ConfigSchema is the ordered field contract used by validation and forms.
type ConfigSchema struct {
	Fields []ConfigField `json:"fields"`
}

// ConfigIssue identifies one invalid field value.
type ConfigIssue struct {
	Field   string
	Code    string
	Message string
}

// WithDefaults returns a copy of config with declared defaults materialized.
func (s ConfigSchema) WithDefaults(config map[string]any) map[string]any {
	result := make(map[string]any, len(config)+len(s.Fields))
	for name, value := range config {
		result[name] = value
	}
	for _, field := range s.Fields {
		if _, exists := result[field.Name]; !exists && field.HasDefault {
			result[field.Name] = field.Default
		}
	}
	return result
}

// Validate returns all config issues in schema order, followed by unknown fields.
func (s ConfigSchema) Validate(config map[string]any) []ConfigIssue {
	issues := make([]ConfigIssue, 0)
	known := make(map[string]struct{}, len(s.Fields))
	for _, field := range s.Fields {
		known[field.Name] = struct{}{}
		value, exists := config[field.Name]
		if !exists || value == nil {
			if field.Required && !field.HasDefault {
				issues = append(issues, ConfigIssue{Field: field.Name, Code: "required", Message: "field is required"})
			}
			continue
		}
		if issue := validateValue(field, value); issue != nil {
			issues = append(issues, *issue)
		}
	}
	unknown := make([]string, 0)
	for name := range config {
		if _, exists := known[name]; !exists {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		issues = append(issues, ConfigIssue{Field: name, Code: "unknown", Message: "field is not declared by the Node Definition"})
	}
	return issues
}

func validateValue(field ConfigField, value any) *ConfigIssue {
	issue := func(code, message string) *ConfigIssue {
		return &ConfigIssue{Field: field.Name, Code: code, Message: message}
	}
	switch field.Type {
	case FieldString, FieldMarkdown:
		if _, ok := value.(string); !ok {
			return issue("invalid-type", fmt.Sprintf("must be %s", field.Type))
		}
	case FieldBoolean:
		if _, ok := value.(bool); !ok {
			return issue("invalid-type", "must be boolean")
		}
	case FieldInteger:
		number, ok := numericValue(value)
		if !ok || math.Trunc(number) != number {
			return issue("invalid-type", "must be integer")
		}
		if rangeIssue := validateRange(field, number); rangeIssue != nil {
			return rangeIssue
		}
	case FieldNumber:
		number, ok := numericValue(value)
		if !ok {
			return issue("invalid-type", "must be number")
		}
		if rangeIssue := validateRange(field, number); rangeIssue != nil {
			return rangeIssue
		}
	case FieldEnum:
		text, ok := value.(string)
		if !ok {
			return issue("invalid-type", "must be enum value")
		}
		for _, allowed := range field.Values {
			if text == allowed {
				return nil
			}
		}
		return issue("invalid-enum", fmt.Sprintf("must be one of %v", field.Values))
	default:
		return issue("invalid-schema", fmt.Sprintf("unsupported field type %q", field.Type))
	}
	return nil
}

func validateRange(field ConfigField, number float64) *ConfigIssue {
	if field.Min != nil && number < *field.Min {
		return &ConfigIssue{Field: field.Name, Code: "below-minimum", Message: fmt.Sprintf("must be at least %v", *field.Min)}
	}
	if field.Max != nil && number > *field.Max {
		return &ConfigIssue{Field: field.Name, Code: "above-maximum", Message: fmt.Sprintf("must be at most %v", *field.Max)}
	}
	return nil
}

func numericValue(value any) (float64, bool) {
	switch number := value.(type) {
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case float64:
		return number, !math.IsNaN(number) && !math.IsInf(number, 0)
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	default:
		return 0, false
	}
}
