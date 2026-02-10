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
	"context"
	"fmt"
	"strings"

	"github.com/grafana/grafana-openapi-client-go/client/sso_settings"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/pkg/errors"
)

const ssoProvider = "generic_oauth"

// TenantMapping holds the fields needed to produce org_mapping entries for a tenant.
type TenantMapping struct {
	OrgID        string
	ViewerGroups []string
	EditorGroups []string
}

// SSOClient is the subset of the Grafana SSO settings API used by this package.
type SSOClient interface {
	GetProviderSettings(key string, opts ...sso_settings.ClientOption) (*sso_settings.GetProviderSettingsOK, error)
	UpdateProviderSettings(key string, body *models.UpdateProviderSettingsParamsBody, opts ...sso_settings.ClientOption) (*sso_settings.UpdateProviderSettingsNoContent, error)
}

// SyncOrgMapping reads the current SSO settings for generic_oauth, computes the
// org_mapping from all tenants, and writes the updated settings back.
func SyncOrgMapping(_ context.Context, ssoc SSOClient, tenants []TenantMapping) error {
	settings, err := getOrInitSettings(ssoc)
	if err != nil {
		return errors.Wrap(err, "cannot get SSO settings")
	}

	settings["orgMapping"] = BuildOrgMapping(tenants)

	body := &models.UpdateProviderSettingsParamsBody{
		Provider: ssoProvider,
		Settings: settings,
	}
	if _, err := ssoc.UpdateProviderSettings(ssoProvider, body); err != nil {
		return errors.Wrap(err, "cannot update SSO settings")
	}
	return nil
}

// OrgMappingContains checks whether the given org_mapping string contains an
// entry for the specified orgId.
func OrgMappingContains(orgMapping, orgID string) bool {
	entry := fmt.Sprintf("%s:%s:Viewer", orgID, orgID)
	for _, part := range strings.Split(orgMapping, ",") {
		if strings.TrimSpace(part) == entry {
			return true
		}
	}
	return false
}

// BuildOrgMapping produces the comma-separated org_mapping value from a set of
// tenant mappings. For each tenant it emits:
//   - <orgId>:<orgId>:Viewer  (the default entry)
//   - <group>:<orgId>:Viewer  for each ViewerGroup
//   - <group>:<orgId>:Editor  for each EditorGroup
//
// Group names containing colons are automatically escaped with \: to prevent
// parsing issues in Grafana's org_mapping format.
func BuildOrgMapping(tenants []TenantMapping) string {
	entries := make([]string, 0, len(tenants))
	for _, t := range tenants {
		for _, g := range t.ViewerGroups {
			entries = append(entries, fmt.Sprintf("%s:%s:Viewer", escapeColon(g), t.OrgID))
		}
		for _, g := range t.EditorGroups {
			entries = append(entries, fmt.Sprintf("%s:%s:Editor", escapeColon(g), t.OrgID))
		}
	}
	return strings.Join(entries, ",")
}

// escapeColon escapes colons in a string for use in Grafana org_mapping.
// Grafana uses : as a delimiter, so literal colons in group names must be
// escaped as \: to be parsed correctly.
func escapeColon(s string) string {
	return strings.ReplaceAll(s, ":", `\:`)
}

// getOrInitSettings fetches the current SSO settings for generic_oauth.
// If the provider returns 404, an empty settings map is returned.
func getOrInitSettings(ssoc SSOClient) (map[string]interface{}, error) {
	resp, err := ssoc.GetProviderSettings(ssoProvider)
	if err != nil {
		// If the provider is not configured yet, start with an empty map.
		if IsNotFound(err) {
			return map[string]interface{}{}, nil
		}
		return nil, err
	}

	m, ok := resp.Payload.Settings.(map[string]interface{})
	if !ok {
		return nil, errors.New("SSO settings is not a map")
	}
	return m, nil
}

// IsNotFound returns true when the error represents a 404 response.
func IsNotFound(err error) bool {
	var notFound *sso_settings.GetProviderSettingsNotFound
	return errors.As(err, &notFound)
}
