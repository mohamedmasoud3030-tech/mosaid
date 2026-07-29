package mcpclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const maxSchemaBytes = 64 * 1024

func validateToolSchema(schema any) error {
	encoded, err := json.Marshal(schema)
	if err != nil || len(encoded) == 0 || len(encoded) > maxSchemaBytes {
		return fmt.Errorf("%w: schema encoding or size", ErrSchemaRejected)
	}
	var value any
	if err = json.Unmarshal(encoded, &value); err != nil {
		return fmt.Errorf("%w: invalid JSON", ErrSchemaRejected)
	}
	object, ok := value.(map[string]any)
	if !ok || object["type"] != "object" {
		return fmt.Errorf("%w: root type must be object", ErrSchemaRejected)
	}
	if additional, exists := object["additionalProperties"]; exists && additional != false {
		return fmt.Errorf("%w: open-ended properties denied", ErrSchemaRejected)
	}
	properties, exists := object["properties"]
	if !exists {
		return fmt.Errorf("%w: explicit properties required", ErrSchemaRejected)
	}
	if propertyMap, ok := properties.(map[string]any); !ok || len(propertyMap) > 128 {
		return fmt.Errorf("%w: properties malformed or excessive", ErrSchemaRejected)
	}
	nodes := 0
	if err = inspectSchema(value, 0, &nodes); err != nil {
		return err
	}
	return nil
}

func inspectSchema(value any, depth int, nodes *int) error {
	*nodes++
	if depth > 16 || *nodes > 1024 {
		return fmt.Errorf("%w: schema complexity limit", ErrSchemaRejected)
	}
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			lower := strings.ToLower(key)
			if key == "$ref" || key == "$dynamicRef" || lower == "contentencoding" || lower == "contentmediatype" {
				return fmt.Errorf("%w: references and encoded content are forbidden", ErrSchemaRejected)
			}
			if len(key) > 256 {
				return fmt.Errorf("%w: key length", ErrSchemaRejected)
			}
			if err := inspectSchema(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case []any:
		if len(item) > 256 {
			return fmt.Errorf("%w: array length", ErrSchemaRejected)
		}
		for _, child := range item {
			if err := inspectSchema(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case string:
		if len(item) > 8192 {
			return fmt.Errorf("%w: string length", ErrSchemaRejected)
		}
	case nil, bool, float64:
	default:
		return errors.New("unsupported schema value")
	}
	return nil
}
