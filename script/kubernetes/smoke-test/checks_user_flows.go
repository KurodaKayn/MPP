package main

import (
	"fmt"
	"strings"
)

func (suite *Suite) publicGateway() {
	suite.reporter.Section("Public Gateway")
	if !suite.config.PublicURLConfigured() {
		suite.missingOptionalInput("public frontend", "set --public-url or MPP_PUBLIC_URL to probe the public frontend")
		return
	}

	suite.check("public frontend root", true, func() (string, error) {
		response, err := suite.http.Get(suite.config.PublicURL+"/", nil)
		if err != nil {
			return "", err
		}
		if err := assertStatus(response, []int{200, 301, 302, 307, 308}, "public root"); err != nil {
			return "", err
		}
		return fmt.Sprintf("status=%d", response.Status), nil
	})

	suite.check("public frontend readiness", true, func() (string, error) {
		response, err := suite.http.Get(suite.config.PublicURL+"/api/ready", nil)
		if err != nil {
			return "", err
		}
		if err := assertStatus(response, []int{200}, "public frontend readiness"); err != nil {
			return "", err
		}
		if err := assert(strings.Contains(response.Body, "ready") || strings.Contains(response.Body, "healthy"), "unexpected readiness body"); err != nil {
			return "", err
		}
		return fmt.Sprintf("status=%d", response.Status), nil
	})
}

func (suite *Suite) authenticatedUserFlows() {
	suite.reporter.Section("Authenticated User Flows")
	if !suite.config.RunUserFlowProbes {
		suite.reporter.Skip("authenticated flow probes", "pass --run-user-flow-probes to enable")
		return
	}
	if !suite.config.UserFlowInputsConfigured() {
		suite.missingOptionalInput("authenticated flow probes", "requires --api-base-url or --public-url plus --auth-token")
		return
	}

	suite.check("authenticated dashboard session", true, func() (string, error) {
		response, err := suite.apiGet("/api/user/dashboard/stats")
		if err != nil {
			return "", err
		}
		if err := assertStatus(response, []int{200}, "dashboard stats"); err != nil {
			return "", err
		}
		return fmt.Sprintf("status=%d", response.Status), nil
	})

	suite.check("project list query", true, func() (string, error) {
		response, err := suite.apiGet("/api/user/dashboard/projects")
		if err != nil {
			return "", err
		}
		if err := assertStatus(response, []int{200}, "project list"); err != nil {
			return "", err
		}
		return fmt.Sprintf("status=%d", response.Status), nil
	})

	suite.projectScopedUserFlows()
	if suite.config.RunBrowserSessionProbe {
		suite.browserSessionProbe()
	}
}

func (suite *Suite) projectScopedUserFlows() {
	if !suite.config.ProjectConfigured() {
		suite.missingOptionalInput("project-scoped collaboration and publishing probes", "set --project-id or MPP_SMOKE_PROJECT_ID to enable")
		return
	}

	suite.check("collaboration session creation", true, func() (string, error) {
		response, err := suite.apiPost("/api/user/dashboard/projects/"+suite.config.ProjectID+"/collab/session", nil)
		if err != nil {
			return "", err
		}
		if err := assertStatus(response, []int{200, 201}, "collaboration session"); err != nil {
			return "", err
		}
		body, err := parseJSON(response.Body)
		if err != nil {
			return "", err
		}
		if err := assertPresent(body["token"], "collaboration session response is missing token"); err != nil {
			return "", err
		}
		if err := assertPresent(body["document_id"], "collaboration session response is missing document_id"); err != nil {
			return "", err
		}
		return "document_id=" + stringValue(body["document_id"]), nil
	})

	suite.check("publishing dependency read path", true, func() (string, error) {
		response, err := suite.apiGet("/api/user/dashboard/projects/" + suite.config.ProjectID + "/publications")
		if err != nil {
			return "", err
		}
		if err := assertStatus(response, []int{200}, "project publications"); err != nil {
			return "", err
		}
		return fmt.Sprintf("status=%d", response.Status), nil
	})
}

func (suite *Suite) browserSessionProbe() {
	suite.check("remote browser session lifecycle", true, func() (string, error) {
		sessionID := ""
		defer func() {
			if sessionID != "" {
				_, _ = suite.apiDelete("/api/user/dashboard/browser-sessions/" + sessionID)
			}
		}()

		start, err := suite.apiPost("/api/user/dashboard/settings/platforms/"+suite.config.BrowserPlatform+"/browser-session", nil)
		if err != nil {
			return "", err
		}
		if err := assertStatus(start, []int{200, 201}, "browser session start"); err != nil {
			return "", err
		}
		body, err := parseJSON(start.Body)
		if err != nil {
			return "", err
		}
		sessionID = firstPresent(body, "session_id", "sessionId", "id")
		if err := assertPresent(sessionID, "browser session response is missing session_id"); err != nil {
			return "", err
		}

		status, err := suite.apiGet("/api/user/dashboard/browser-sessions/" + sessionID)
		if err != nil {
			return "", err
		}
		if err := assertStatus(status, []int{200}, "browser session status"); err != nil {
			return "", err
		}

		cancelled, err := suite.apiDelete("/api/user/dashboard/browser-sessions/" + sessionID)
		if err != nil {
			return "", err
		}
		if err := assertStatus(cancelled, []int{200, 202, 204}, "browser session cancel"); err != nil {
			return "", err
		}
		return "session_id=" + sessionID + " cancelled", nil
	})
}

func (suite *Suite) missingOptionalInput(name string, message string) {
	if suite.config.RequireUserFlows {
		suite.check(name, true, func() (string, error) {
			return "", CheckFailure(message)
		})
		return
	}
	suite.reporter.Skip(name, message)
}

func (suite *Suite) apiGet(path string) (Response, error) {
	return suite.http.Get(suite.apiURL(path), suite.authHeaders())
}

func (suite *Suite) apiPost(path string, jsonBody any) (Response, error) {
	return suite.http.Post(suite.apiURL(path), suite.authHeaders(), jsonBody)
}

func (suite *Suite) apiDelete(path string) (Response, error) {
	return suite.http.Delete(suite.apiURL(path), suite.authHeaders())
}

func (suite *Suite) apiURL(path string) string {
	return suite.config.APIBaseURL + path
}

func (suite *Suite) authHeaders() map[string]string {
	return map[string]string{"Authorization": "Bearer " + suite.config.AuthToken}
}
