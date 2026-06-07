package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

func as[T error](err error, target *T) bool {
	return errors.As(err, target)
}

func failure(format string, args ...any) error {
	return CheckFailure(fmt.Sprintf(format, args...))
}

func assert(condition bool, message string) error {
	if condition {
		return nil
	}
	return CheckFailure(message)
}

func assertEqual(expected string, actual string, message string) error {
	if expected == actual {
		return nil
	}
	return CheckFailure(fmt.Sprintf("%s: expected %q, got %q", message, expected, actual))
}

func assertPresent(value any, message string) error {
	if strings.TrimSpace(stringValue(value)) != "" {
		return nil
	}
	return CheckFailure(message)
}

func dig(object any, path ...string) any {
	current := object
	for _, key := range path {
		asMap, ok := current.(map[string]any)
		if !ok {
			if objectMap, objectOk := current.(Object); objectOk {
				asMap = objectMap
				ok = true
			}
		}
		if !ok {
			return nil
		}
		current = asMap[key]
	}
	return current
}

func asObject(value any) Object {
	if object, ok := value.(Object); ok {
		return object
	}
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return Object{}
}

func asSlice(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	if values, ok := value.([]Object); ok {
		result := make([]any, 0, len(values))
		for _, value := range values {
			result = append(result, value)
		}
		return result
	}
	return nil
}

func asObjectSlice(value any) []Object {
	values := asSlice(value)
	result := make([]Object, 0, len(values))
	for _, value := range values {
		result = append(result, asObject(value))
	}
	return result
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func sortedKeys(object Object) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
