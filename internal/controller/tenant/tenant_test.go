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

package tenant

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"

	"github.com/grafana/grafana-openapi-client-go/client/sso_settings"
	"github.com/grafana/grafana-openapi-client-go/models"

	v1alpha1 "github.com/loafoe/provider-orgmapper/apis/tenant/v1alpha1"
	"github.com/loafoe/provider-orgmapper/internal/grafana"
)

// mockSSO implements grafana.SSOClient for controller tests.
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

// defaultMockSSO returns a mock that reports the expected orgMapping for the
// given orgIDs. Each org gets a default viewer group entry.
func defaultMockSSO(orgIDs ...string) *mockSSO {
	tenants := make([]grafana.TenantMapping, 0, len(orgIDs))
	for _, id := range orgIDs {
		// Add a default viewer group so drift detection works
		tenants = append(tenants, grafana.TenantMapping{
			OrgID:        id,
			ViewerGroups: []string{"default-viewers"},
		})
	}
	return &mockSSO{
		getResp: &sso_settings.GetProviderSettingsOK{
			Payload: &models.GetProviderSettingsOKBody{
				Settings: map[string]any{
					"orgMapping": grafana.BuildOrgMapping(tenants),
				},
			},
		},
	}
}

func tenantWithSpec(tenantID, orgID string, admins []string, retention v1alpha1.RetentionPolicy) *v1alpha1.Tenant {
	t := &v1alpha1.Tenant{}
	t.Spec.ForProvider = v1alpha1.TenantParameters{
		TenantID:  tenantID,
		OrgID:     orgID,
		Admins:    admins,
		Retention: retention,
	}
	return t
}

func newFakeKube(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = v1alpha1.SchemeBuilder.AddToScheme(scheme)
	return clfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()
}

func TestObserve(t *testing.T) {
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		o   managed.ExternalObservation
		err error
	}

	retention := v1alpha1.RetentionPolicy{
		Logs:    "30d",
		Metrics: "90d",
	}

	cases := map[string]struct {
		reason string
		sso    *mockSSO
		args   args
		want   want
	}{
		"NoExternalName": {
			reason: "Should return ResourceExists false when no external name is set.",
			sso:    defaultMockSSO(),
			args: args{
				ctx: context.Background(),
				mg:  tenantWithSpec("acme", "org-1", nil, retention),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: false},
			},
		},
		"ExternalNameSetButNoStatus": {
			reason: "Should return ResourceExists false when external name is set but status is empty.",
			sso:    defaultMockSSO(),
			args: args{
				ctx: context.Background(),
				mg: func() resource.Managed {
					cr := tenantWithSpec("acme", "org-1", nil, retention)
					meta.SetExternalName(cr, "acme")
					return cr
				}(),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false},
			},
		},
		"UpToDate": {
			reason: "Should return ResourceExists true and ResourceUpToDate true when spec matches status.",
			sso:    defaultMockSSO("org-1"),
			args: args{
				ctx: context.Background(),
				mg: func() resource.Managed {
					cr := tenantWithSpec("acme", "org-1", []string{"admin1"}, retention)
					meta.SetExternalName(cr, "acme")
					cr.Status.AtProvider = v1alpha1.TenantObservation{
						TenantID:    "acme",
						OrgID:       "org-1",
						Admins:      []string{"admin1"},
						Retention:   retention,
						LastUpdated: "2025-01-01T00:00:00Z",
					}
					return cr
				}(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"NotUpToDate": {
			reason: "Should return ResourceUpToDate false when spec diverges from status.",
			sso:    defaultMockSSO("org-1"),
			args: args{
				ctx: context.Background(),
				mg: func() resource.Managed {
					cr := tenantWithSpec("acme", "org-2", nil, retention)
					meta.SetExternalName(cr, "acme")
					cr.Status.AtProvider = v1alpha1.TenantObservation{
						TenantID:    "acme",
						OrgID:       "org-1",
						Retention:   retention,
						LastUpdated: "2025-01-01T00:00:00Z",
					}
					return cr
				}(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
			},
		},
		"GrafanaDriftDetected": {
			reason: "Should return ResourceUpToDate false when Grafana org_mapping drifted, to trigger resync.",
			sso:    defaultMockSSO("org-OTHER"),
			args: args{
				ctx: context.Background(),
				mg: func() resource.Managed {
					cr := tenantWithSpec("acme", "org-1", nil, retention)
					cr.Spec.ForProvider.ViewerGroups = []string{"team-a"} // Has groups, so drift check runs
					meta.SetExternalName(cr, "acme")
					cr.Status.AtProvider = v1alpha1.TenantObservation{
						TenantID:     "acme",
						OrgID:        "org-1",
						ViewerGroups: []string{"team-a"},
						Retention:    retention,
						LastUpdated:  "2025-01-01T00:00:00Z",
					}
					return cr
				}(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
			},
		},
		"NotATenant": {
			reason: "Should return an error if the managed resource is not a Tenant.",
			sso:    defaultMockSSO(),
			args: args{
				ctx: context.Background(),
				mg:  &fake.Managed{},
			},
			want: want{
				err: errNotTenantError(),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{sso: tc.sso, logger: logging.NewNopLogger()}
			got, err := e.Observe(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		o   managed.ExternalCreation
		err error
	}

	retention := v1alpha1.RetentionPolicy{Logs: "30d"}

	cases := map[string]struct {
		reason string
		kube   client.Client
		sso    *mockSSO
		args   args
		want   want
	}{
		"Success": {
			reason: "Should set external name, populate status, and sync Grafana.",
			kube:   newFakeKube(),
			sso:    defaultMockSSO(),
			args: args{
				ctx: context.Background(),
				mg:  tenantWithSpec("acme", "org-1", []string{"admin1"}, retention),
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"NotATenant": {
			reason: "Should return an error if the managed resource is not a Tenant.",
			sso:    defaultMockSSO(),
			args: args{
				ctx: context.Background(),
				mg:  &fake.Managed{},
			},
			want: want{
				err: errNotTenantError(),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{kube: tc.kube, sso: tc.sso, logger: logging.NewNopLogger()}
			got, err := e.Create(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want, +got:\n%s\n", tc.reason, diff)
			}

			// Verify side effects for successful cases.
			if err == nil {
				cr, ok := tc.args.mg.(*v1alpha1.Tenant)
				if ok {
					if meta.GetExternalName(cr) != cr.Spec.ForProvider.TenantID {
						t.Errorf("\n%s\ne.Create(...): expected external name %q, got %q", tc.reason, cr.Spec.ForProvider.TenantID, meta.GetExternalName(cr))
					}
					if cr.Status.AtProvider.TenantID != cr.Spec.ForProvider.TenantID {
						t.Errorf("\n%s\ne.Create(...): expected status tenantId %q, got %q", tc.reason, cr.Spec.ForProvider.TenantID, cr.Status.AtProvider.TenantID)
					}
					if cr.Status.AtProvider.LastUpdated == "" {
						t.Errorf("\n%s\ne.Create(...): expected lastUpdated to be set", tc.reason)
					}
				}
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		o   managed.ExternalUpdate
		err error
	}

	cases := map[string]struct {
		reason string
		kube   client.Client
		sso    *mockSSO
		args   args
		want   want
	}{
		"Success": {
			reason: "Should sync changed spec to status, update timestamp, and sync Grafana.",
			kube:   newFakeKube(),
			sso:    defaultMockSSO(),
			args: args{
				ctx: context.Background(),
				mg: func() resource.Managed {
					cr := tenantWithSpec("acme", "org-2", nil, v1alpha1.RetentionPolicy{Logs: "60d"})
					meta.SetExternalName(cr, "acme")
					cr.Status.AtProvider = v1alpha1.TenantObservation{
						TenantID:    "acme",
						OrgID:       "org-1",
						Retention:   v1alpha1.RetentionPolicy{Logs: "30d"},
						LastUpdated: "2025-01-01T00:00:00Z",
					}
					return cr
				}(),
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{kube: tc.kube, sso: tc.sso, logger: logging.NewNopLogger()}
			got, err := e.Update(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want, +got:\n%s\n", tc.reason, diff)
			}

			// Verify side effects.
			if err == nil {
				cr, ok := tc.args.mg.(*v1alpha1.Tenant)
				if ok {
					if cr.Status.AtProvider.OrgID != cr.Spec.ForProvider.OrgID {
						t.Errorf("\n%s\ne.Update(...): expected status orgId %q, got %q", tc.reason, cr.Spec.ForProvider.OrgID, cr.Status.AtProvider.OrgID)
					}
					if cr.Status.AtProvider.Retention != cr.Spec.ForProvider.Retention {
						t.Errorf("\n%s\ne.Update(...): expected retention to match spec", tc.reason)
					}
				}
			}
		})
	}
}

func TestDelete(t *testing.T) {
	cr := tenantWithSpec("acme", "org-1", nil, v1alpha1.RetentionPolicy{})
	e := external{kube: newFakeKube(), sso: defaultMockSSO(), logger: logging.NewNopLogger()}
	got, err := e.Delete(context.Background(), cr)
	if err != nil {
		t.Errorf("e.Delete(...): unexpected error: %v", err)
	}
	if diff := cmp.Diff(managed.ExternalDelete{}, got); diff != "" {
		t.Errorf("e.Delete(...): -want, +got:\n%s", diff)
	}
}

func TestIsUpToDate(t *testing.T) {
	cases := map[string]struct {
		reason string
		cr     *v1alpha1.Tenant
		want   bool
	}{
		"UpToDate": {
			reason: "Should return true when all fields match.",
			cr: func() *v1alpha1.Tenant {
				cr := tenantWithSpec("acme", "org-1", []string{"admin1"}, v1alpha1.RetentionPolicy{Logs: "30d"})
				cr.Status.AtProvider = v1alpha1.TenantObservation{
					TenantID:  "acme",
					OrgID:     "org-1",
					Admins:    []string{"admin1"},
					Retention: v1alpha1.RetentionPolicy{Logs: "30d"},
				}
				return cr
			}(),
			want: true,
		},
		"DifferentOrgID": {
			reason: "Should return false when orgId differs.",
			cr: func() *v1alpha1.Tenant {
				cr := tenantWithSpec("acme", "org-2", nil, v1alpha1.RetentionPolicy{})
				cr.Status.AtProvider = v1alpha1.TenantObservation{
					TenantID: "acme",
					OrgID:    "org-1",
				}
				return cr
			}(),
			want: false,
		},
		"DifferentRetention": {
			reason: "Should return false when retention differs.",
			cr: func() *v1alpha1.Tenant {
				cr := tenantWithSpec("acme", "org-1", nil, v1alpha1.RetentionPolicy{Logs: "60d"})
				cr.Status.AtProvider = v1alpha1.TenantObservation{
					TenantID:  "acme",
					OrgID:     "org-1",
					Retention: v1alpha1.RetentionPolicy{Logs: "30d"},
				}
				return cr
			}(),
			want: false,
		},
		"NilVsEmptyAdmins": {
			reason: "Should treat nil and empty admin slices as equivalent.",
			cr: func() *v1alpha1.Tenant {
				cr := tenantWithSpec("acme", "org-1", nil, v1alpha1.RetentionPolicy{})
				cr.Status.AtProvider = v1alpha1.TenantObservation{
					TenantID: "acme",
					OrgID:    "org-1",
					Admins:   []string{},
				}
				return cr
			}(),
			want: true,
		},
		"DifferentAdmins": {
			reason: "Should return false when admins differ.",
			cr: func() *v1alpha1.Tenant {
				cr := tenantWithSpec("acme", "org-1", []string{"admin1"}, v1alpha1.RetentionPolicy{})
				cr.Status.AtProvider = v1alpha1.TenantObservation{
					TenantID: "acme",
					OrgID:    "org-1",
					Admins:   []string{"admin2"},
				}
				return cr
			}(),
			want: false,
		},
		"DifferentViewerGroups": {
			reason: "Should return false when viewerGroups differ.",
			cr: func() *v1alpha1.Tenant {
				cr := tenantWithSpec("acme", "org-1", nil, v1alpha1.RetentionPolicy{})
				cr.Spec.ForProvider.ViewerGroups = []string{"team-a"}
				cr.Status.AtProvider = v1alpha1.TenantObservation{
					TenantID:     "acme",
					OrgID:        "org-1",
					ViewerGroups: []string{"team-b"},
				}
				return cr
			}(),
			want: false,
		},
		"DifferentEditorGroups": {
			reason: "Should return false when editorGroups differ.",
			cr: func() *v1alpha1.Tenant {
				cr := tenantWithSpec("acme", "org-1", nil, v1alpha1.RetentionPolicy{})
				cr.Spec.ForProvider.EditorGroups = []string{"devs"}
				cr.Status.AtProvider = v1alpha1.TenantObservation{
					TenantID: "acme",
					OrgID:    "org-1",
				}
				return cr
			}(),
			want: false,
		},
		"MatchingGroups": {
			reason: "Should return true when viewerGroups and editorGroups match.",
			cr: func() *v1alpha1.Tenant {
				cr := tenantWithSpec("acme", "org-1", nil, v1alpha1.RetentionPolicy{})
				cr.Spec.ForProvider.ViewerGroups = []string{"team-a"}
				cr.Spec.ForProvider.EditorGroups = []string{"devs"}
				cr.Status.AtProvider = v1alpha1.TenantObservation{
					TenantID:     "acme",
					OrgID:        "org-1",
					ViewerGroups: []string{"team-a"},
					EditorGroups: []string{"devs"},
				}
				return cr
			}(),
			want: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := isUpToDate(tc.cr)
			if got != tc.want {
				t.Errorf("\n%s\nisUpToDate(...): want %v, got %v", tc.reason, tc.want, got)
			}
		})
	}
}

func errNotTenantError() error {
	return errors.New(errNotTenant)
}
