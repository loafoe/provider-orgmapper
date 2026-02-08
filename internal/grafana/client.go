/*
Copyright 2025 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package grafana

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/go-openapi/strfmt"
	goapi "github.com/grafana/grafana-openapi-client-go/client"
	"github.com/pkg/errors"
)

// basicAuthCreds is the JSON structure for basic auth credentials.
type basicAuthCreds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// NewClient creates a Grafana HTTP API client from the given URL and raw credentials.
// If creds is JSON with "username" and "password" keys, basic auth is used.
// Otherwise creds is treated as a bearer token string.
func NewClient(grafanaURL string, creds []byte) (*goapi.GrafanaHTTPAPI, error) {
	u, err := url.Parse(grafanaURL)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse grafana URL")
	}

	cfg := &goapi.TransportConfig{
		Host:     u.Host,
		BasePath: basePath(u.Path),
		Schemes:  []string{u.Scheme},
	}

	token := strings.TrimSpace(string(creds))

	var ba basicAuthCreds
	if json.Unmarshal(creds, &ba) == nil && ba.Username != "" && ba.Password != "" {
		cfg.BasicAuth = url.UserPassword(ba.Username, ba.Password)
	} else {
		cfg.APIKey = token
	}

	return goapi.NewHTTPClientWithConfig(strfmt.Default, cfg), nil
}

// basePath ensures the path ends with /api.
func basePath(path string) string {
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "/api"
	}
	if strings.HasSuffix(path, "/api") {
		return path
	}
	return path + "/api"
}
