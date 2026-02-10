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
	"testing"

	"github.com/grafana/grafana-openapi-client-go/client/sso_settings"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/pkg/errors"
)

// mockSSO implements SSOClient for testing.
type mockSSO struct {
	getResp *sso_settings.GetProviderSettingsOK
	getErr  error
	putBody *models.UpdateProviderSettingsParamsBody
	putErr  error
}

func (m *mockSSO) GetProviderSettings(key string, _ ...sso_settings.ClientOption) (*sso_settings.GetProviderSettingsOK, error) {
	return m.getResp, m.getErr
}

func (m *mockSSO) UpdateProviderSettings(key string, body *models.UpdateProviderSettingsParamsBody, _ ...sso_settings.ClientOption) (*sso_settings.UpdateProviderSettingsNoContent, error) {
	m.putBody = body
	if m.putErr != nil {
		return nil, m.putErr
	}
	return &sso_settings.UpdateProviderSettingsNoContent{}, nil
}

func TestBuildOrgMapping(t *testing.T) {
	cases := map[string]struct {
		tenants []TenantMapping
		want    string
	}{
		"Empty": {
			tenants: nil,
			want:    "",
		},
		"SingleTenant": {
			// Tenant without groups produces no entries
			tenants: []TenantMapping{{OrgID: "org-1"}},
			want:    "",
		},
		"MultipleTenants": {
			// Tenants without groups produce no entries
			tenants: []TenantMapping{
				{OrgID: "org-1"},
				{OrgID: "org-2"},
				{OrgID: "org-3"},
			},
			want: "",
		},
		"WithViewerGroups": {
			tenants: []TenantMapping{
				{OrgID: "org-1", ViewerGroups: []string{"team-a", "team-b"}},
			},
			want: "team-a:org-1:Viewer,team-b:org-1:Viewer",
		},
		"WithEditorGroups": {
			tenants: []TenantMapping{
				{OrgID: "org-1", EditorGroups: []string{"devs"}},
			},
			want: "devs:org-1:Editor",
		},
		"WithViewerAndEditorGroups": {
			tenants: []TenantMapping{
				{OrgID: "org-1", ViewerGroups: []string{"readers"}, EditorGroups: []string{"writers"}},
				{OrgID: "org-2"}, // No groups, no entry
			},
			want: "readers:org-1:Viewer,writers:org-1:Editor",
		},
		"WithColonsInGroupNames": {
			tenants: []TenantMapping{
				{OrgID: "org-1", ViewerGroups: []string{"oidc:team:viewers"}, EditorGroups: []string{"oidc:team:editors"}},
			},
			want: `oidc\:team\:viewers:org-1:Viewer,oidc\:team\:editors:org-1:Editor`,
		},
		"WithMixedGroupNames": {
			tenants: []TenantMapping{
				{OrgID: "org-1", ViewerGroups: []string{"simple-group", "ns:complex:group"}, EditorGroups: []string{"editors"}},
			},
			want: `simple-group:org-1:Viewer,ns\:complex\:group:org-1:Viewer,editors:org-1:Editor`,
		},
		"WithAdminGroups": {
			tenants: []TenantMapping{
				{OrgID: "org-1", AdminGroups: []string{"platform-admins"}},
			},
			want: "platform-admins:org-1:Admin",
		},
		"WithAllGroupTypes": {
			tenants: []TenantMapping{
				{OrgID: "org-1", ViewerGroups: []string{"readers"}, EditorGroups: []string{"writers"}, AdminGroups: []string{"admins"}},
			},
			want: "readers:org-1:Viewer,writers:org-1:Editor,admins:org-1:Admin",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := BuildOrgMapping(tc.tenants)
			if got != tc.want {
				t.Errorf("BuildOrgMapping(...) = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOrgMappingContains(t *testing.T) {
	cases := map[string]struct {
		orgMapping string
		orgID      string
		want       bool
	}{
		"Present": {
			orgMapping: "team-a:org-1:Viewer,team-b:org-2:Viewer",
			orgID:      "org-1",
			want:       true,
		},
		"Absent": {
			orgMapping: "team-a:org-1:Viewer,team-b:org-2:Viewer",
			orgID:      "org-3",
			want:       false,
		},
		"Empty": {
			orgMapping: "",
			orgID:      "org-1",
			want:       false,
		},
		"SingleMatch": {
			orgMapping: "viewers:org-1:Viewer",
			orgID:      "org-1",
			want:       true,
		},
		"MultipleEntriesForSameOrg": {
			orgMapping: "team-a:org-1:Viewer,team-b:org-1:Editor",
			orgID:      "org-1",
			want:       true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := OrgMappingContains(tc.orgMapping, tc.orgID)
			if got != tc.want {
				t.Errorf("OrgMappingContains(%q, %q) = %v, want %v", tc.orgMapping, tc.orgID, got, tc.want)
			}
		})
	}
}

func TestSyncOrgMapping(t *testing.T) {
	cases := map[string]struct {
		mock    *mockSSO
		tenants []TenantMapping
		wantErr bool
		wantMap string
	}{
		"Success": {
			mock: &mockSSO{
				getResp: &sso_settings.GetProviderSettingsOK{
					Payload: &models.GetProviderSettingsOKBody{
						Settings: map[string]any{
							"clientId":     "my-client",
							"clientSecret": "my-secret",
						},
					},
				},
			},
			tenants: []TenantMapping{
				{OrgID: "org-1", ViewerGroups: []string{"team-a"}, AdminGroups: []string{"admins-a"}},
				{OrgID: "org-2", ViewerGroups: []string{"team-b"}},
			},
			wantMap: "team-a:org-1:Viewer,admins-a:org-1:Admin,team-b:org-2:Viewer",
		},
		"NotFoundCreatesNew": {
			mock: &mockSSO{
				getErr: &sso_settings.GetProviderSettingsNotFound{},
			},
			tenants: []TenantMapping{{OrgID: "org-1", ViewerGroups: []string{"viewers"}}},
			wantMap: "viewers:org-1:Viewer",
		},
		"GetError": {
			mock: &mockSSO{
				getErr: errors.New("connection refused"),
			},
			wantErr: true,
		},
		"PutError": {
			mock: &mockSSO{
				getResp: &sso_settings.GetProviderSettingsOK{
					Payload: &models.GetProviderSettingsOKBody{
						Settings: map[string]any{},
					},
				},
				putErr: errors.New("forbidden"),
			},
			tenants: []TenantMapping{{OrgID: "org-1", ViewerGroups: []string{"viewers"}}},
			wantErr: true,
		},
		"PreservesExistingSettings": {
			mock: &mockSSO{
				getResp: &sso_settings.GetProviderSettingsOK{
					Payload: &models.GetProviderSettingsOKBody{
						Settings: map[string]any{
							"clientId":     "my-client",
							"clientSecret": "my-secret",
							"orgMapping":   "old:old:Viewer",
						},
					},
				},
			},
			tenants: []TenantMapping{{OrgID: "org-1", ViewerGroups: []string{"new-team"}}},
			wantMap: "new-team:org-1:Viewer",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := SyncOrgMapping(context.Background(), tc.mock, tc.tenants)
			if tc.wantErr {
				if err == nil {
					t.Error("SyncOrgMapping(...): expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("SyncOrgMapping(...): unexpected error: %v", err)
				return
			}

			if tc.mock.putBody == nil {
				t.Fatal("SyncOrgMapping(...): expected UpdateProviderSettings to be called")
			}

			settings, ok := tc.mock.putBody.Settings.(map[string]any)
			if !ok {
				t.Fatal("SyncOrgMapping(...): settings is not a map")
			}

			gotMapping, _ := settings["orgMapping"].(string)
			if gotMapping != tc.wantMap {
				t.Errorf("SyncOrgMapping(...): orgMapping = %q, want %q", gotMapping, tc.wantMap)
			}

			// Verify existing settings are preserved (not just orgMapping).
			if name == "PreservesExistingSettings" {
				if settings["clientId"] != "my-client" {
					t.Error("SyncOrgMapping(...): expected clientId to be preserved")
				}
				if settings["clientSecret"] != "my-secret" {
					t.Error("SyncOrgMapping(...): expected clientSecret to be preserved")
				}
			}
		})
	}
}
