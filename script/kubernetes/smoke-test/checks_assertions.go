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
	return failure("%s returned HTTP %d: %s", label, response.Status, body)
}

func parseJSON(body string) (Object, error) {
	var result Object
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, CheckFailure("response body is not JSON: " + err.Error())
	}
	return result, nil
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
