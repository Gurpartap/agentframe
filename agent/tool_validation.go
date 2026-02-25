package agent

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
)

func indexToolDefinitions(definitions []ToolDefinition) map[string]ToolDefinition {
	out := make(map[string]ToolDefinition, len(definitions))
	for i := range definitions {
		out[definitions[i].Name] = definitions[i]
	}
	return out
}

func validateToolCallArguments(call ToolCall, definition ToolDefinition) error {
	return validateToolArguments(definition.InputSchema, call.Arguments)
}

func validateToolArguments(schema map[string]any, arguments map[string]any) error {
	if len(schema) == 0 {
		return nil
	}

	required, err := parseRequiredFields(schema["required"])
	if err != nil {
		return err
	}
	for _, field := range required {
		if _, ok := arguments[field]; !ok {
			return fmt.Errorf("missing required argument %q", field)
		}
	}

	properties, hasProperties := asStringAnyMap(schema["properties"])
	additionalAllowed, err := parseAdditionalProperties(schema["additionalProperties"])
	if err != nil {
		return err
	}

	keys := sortedArgumentKeys(arguments)
	for _, key := range keys {
		value := arguments[key]
		propertySchema, hasProperty := properties[key]
		if !hasProperty {
			if hasProperties && !additionalAllowed {
				return fmt.Errorf("unknown argument %q", key)
			}
			continue
		}

		expectedType, hasType, err := parsePropertyType(propertySchema)
		if err != nil {
			return err
		}
		if !hasType {
			continue
		}
		if !matchesToolArgumentType(expectedType, value) {
			return fmt.Errorf("argument %q must be %q", key, expectedType)
		}
	}

	return nil
}

func normalizedToolErrorResult(call ToolCall, reason ToolFailureReason, err error) ToolResult {
	message := string(reason)
	if err != nil {
		message = fmt.Sprintf("%s: %s", reason, err.Error())
	}
	return ToolResult{
		CallID:        call.ID,
		Name:          call.Name,
		Content:       message,
		IsError:       true,
		FailureReason: reason,
	}
}

func parseRequiredFields(raw any) ([]string, error) {
	switch value := raw.(type) {
	case nil:
		return nil, nil
	case []string:
		out := make([]string, len(value))
		copy(out, value)
		return out, nil
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			field, ok := item.(string)
			if !ok {
				return nil, errors.New(`input schema "required" entries must be strings`)
			}
			out = append(out, field)
		}
		return out, nil
	default:
		return nil, errors.New(`input schema "required" must be an array`)
	}
}

func parseAdditionalProperties(raw any) (bool, error) {
	switch value := raw.(type) {
	case nil:
		return true, nil
	case bool:
		return value, nil
	default:
		return false, errors.New(`input schema "additionalProperties" must be a bool`)
	}
}

func parsePropertyType(propertySchema any) (string, bool, error) {
	propertyMap, ok := asStringAnyMap(propertySchema)
	if !ok {
		return "", false, errors.New(`input schema "properties" entries must be objects`)
	}
	rawType, ok := propertyMap["type"]
	if !ok {
		return "", false, nil
	}
	typeName, ok := rawType.(string)
	if !ok {
		return "", false, errors.New(`input schema property "type" must be a string`)
	}
	return typeName, true, nil
}

func asStringAnyMap(raw any) (map[string]any, bool) {
	switch value := raw.(type) {
	case nil:
		return nil, false
	case map[string]any:
		return value, true
	default:
		return nil, false
	}
}

func sortedArgumentKeys(arguments map[string]any) []string {
	keys := make([]string, 0, len(arguments))
	for key := range arguments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func matchesToolArgumentType(expected string, value any) bool {
	switch expected {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		return isNumber(value)
	case "integer":
		return isInteger(value)
	case "object":
		if value == nil {
			return false
		}
		if _, ok := value.(map[string]any); ok {
			return true
		}
		return reflect.TypeOf(value).Kind() == reflect.Map
	case "array":
		if value == nil {
			return false
		}
		kind := reflect.TypeOf(value).Kind()
		return kind == reflect.Array || kind == reflect.Slice
	default:
		return true
	}
}

func isNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	default:
		return false
	}
}

func isInteger(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}
