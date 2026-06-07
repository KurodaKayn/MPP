package main

import (
	"encoding/json"
	"strings"
)

func assertStatus(response Response, allowed []int, label string) error {
	for _, status := range allowed {
		if response.Status == status {
			return nil
		}
	}
	body := response.Body
	if len(body) > 300 {
		body = body[:300]
	}
	if location := strings.TrimSpace(response.Headers.Get("Location")); location != "" {
		return failure("%s returned HTTP %d with redirect Location %q: %s", label, response.Status, location, body)
	}
	return failure("%s returned HTTP %d: %s", label, response.Status, body)
}

func parseJSON(body string) (Object, error) {
	var result Object
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, CheckFailure("response body is not JSON: " + err.Error())
	}
	return result, nil
}

func assertJSONFields(response Response, label string, fields ...string) (Object, error) {
	body, err := parseJSON(response.Body)
	if err != nil {
		return nil, failure("%s returned non-JSON body: %s", label, err)
	}

	missing := make([]string, 0)
	for _, field := range fields {
		if _, ok := body[field]; !ok {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return nil, failure("%s JSON body missing fields: %s", label, strings.Join(missing, ", "))
	}
	return body, nil
}

func firstPresent(object Object, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(stringValue(object[key]))
		if value != "" {
			return value
		}
	}
	return ""
}
