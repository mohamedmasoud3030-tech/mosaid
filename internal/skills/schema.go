package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"
)

const maxSkillInputBytes = 1024 * 1024

func ValidateInput(schemaRaw, inputRaw json.RawMessage) error {
	if err := validateSchemaDocument(schemaRaw); err != nil {
		return fmt.Errorf("%w: invalid trusted schema", ErrInputSchema)
	}
	if len(inputRaw) == 0 || len(inputRaw) > maxSkillInputBytes || !json.Valid(inputRaw) {
		return fmt.Errorf("%w: input must be bounded JSON", ErrInputSchema)
	}
	if err := rejectDuplicateJSONKeys(inputRaw); err != nil {
		return fmt.Errorf("%w: %v", ErrInputSchema, err)
	}
	var schema, input any
	schemaDecoder := json.NewDecoder(bytes.NewReader(schemaRaw))
	schemaDecoder.UseNumber()
	if err := schemaDecoder.Decode(&schema); err != nil {
		return fmt.Errorf("%w: %v", ErrInputSchema, err)
	}
	inputDecoder := json.NewDecoder(bytes.NewReader(inputRaw))
	inputDecoder.UseNumber()
	if err := inputDecoder.Decode(&input); err != nil {
		return fmt.Errorf("%w: %v", ErrInputSchema, err)
	}
	if err := validateValue(schema.(map[string]any), input, 0); err != nil {
		return fmt.Errorf("%w: %v", ErrInputSchema, err)
	}
	return nil
}

func validateValue(schema map[string]any, value any, depth int) error {
	if depth > 32 {
		return fmt.Errorf("input nesting limit exceeded")
	}
	typeName := schema["type"].(string)
	switch typeName {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object")
		}
		properties := schema["properties"].(map[string]any)
		for key := range object {
			if _, exists := properties[key]; !exists {
				return fmt.Errorf("unknown property %q", key)
			}
		}
		if required, exists := schema["required"].([]any); exists {
			for _, item := range required {
				name := item.(string)
				if _, exists := object[name]; !exists {
					return fmt.Errorf("required property %q missing", name)
				}
			}
		}
		for key, item := range object {
			child := properties[key].(map[string]any)
			if err := validateValue(child, item, depth+1); err != nil {
				return fmt.Errorf("property %q: %w", key, err)
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("expected array")
		}
		if err := checkIntegerBound(schema, "minItems", len(array), false); err != nil {
			return err
		}
		if err := checkIntegerBound(schema, "maxItems", len(array), true); err != nil {
			return err
		}
		items := schema["items"].(map[string]any)
		for index, item := range array {
			if err := validateValue(items, item, depth+1); err != nil {
				return fmt.Errorf("item %d: %w", index, err)
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string")
		}
		length := utf8.RuneCountInString(text)
		if err := checkIntegerBound(schema, "minLength", length, false); err != nil {
			return err
		}
		if err := checkIntegerBound(schema, "maxLength", length, true); err != nil {
			return err
		}
	case "number", "integer":
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("expected number")
		}
		parsed, err := strconv.ParseFloat(number.String(), 64)
		if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
			return fmt.Errorf("invalid number")
		}
		if typeName == "integer" && math.Trunc(parsed) != parsed {
			return fmt.Errorf("expected integer")
		}
		if minimum, exists := schema["minimum"].(json.Number); exists {
			bound, _ := strconv.ParseFloat(minimum.String(), 64)
			if parsed < bound {
				return fmt.Errorf("number below minimum")
			}
		}
		if maximum, exists := schema["maximum"].(json.Number); exists {
			bound, _ := strconv.ParseFloat(maximum.String(), 64)
			if parsed > bound {
				return fmt.Errorf("number above maximum")
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean")
		}
	case "null":
		if value != nil {
			return fmt.Errorf("expected null")
		}
	}
	if enum, exists := schema["enum"].([]any); exists {
		match := false
		encoded, _ := json.Marshal(value)
		for _, candidate := range enum {
			candidateEncoded, _ := json.Marshal(candidate)
			if bytes.Equal(encoded, candidateEncoded) {
				match = true
				break
			}
		}
		if !match {
			return fmt.Errorf("value is not in enum")
		}
	}
	return nil
}

func checkIntegerBound(schema map[string]any, name string, value int, maximum bool) error {
	raw, exists := schema[name].(json.Number)
	if !exists {
		return nil
	}
	bound, err := strconv.Atoi(raw.String())
	if err != nil || bound < 0 {
		return fmt.Errorf("invalid %s", name)
	}
	if maximum && value > bound {
		return fmt.Errorf("%s exceeded", name)
	}
	if !maximum && value < bound {
		return fmt.Errorf("%s not met", name)
	}
	return nil
}
