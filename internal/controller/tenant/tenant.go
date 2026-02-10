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
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"

	v1alpha1 "github.com/loafoe/provider-orgmapper/apis/tenant/v1alpha1"
	apisv1alpha1 "github.com/loafoe/provider-orgmapper/apis/v1alpha1"
	"github.com/loafoe/provider-orgmapper/internal/grafana"
)

const (
	errNotTenant    = "managed resource is not a Tenant custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"
	errNewClient    = "cannot create Grafana client"
	errListTenants  = "cannot list Tenants"
)

// SetupGated adds a controller that reconciles Tenant managed resources with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup Tenant controller"))
		}
	}, v1alpha1.TenantGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles Tenant managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.TenantGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithExternalConnector(&connector{
			kube:   mgr.GetClient(),
			usage:  resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			logger: o.Logger,
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	}

	if o.Features.Enabled(feature.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}

	if o.Features.Enabled(feature.EnableAlphaChangeLogs) {
		opts = append(opts, managed.WithChangeLogger(o.ChangeLogOptions.ChangeLogger))
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.TenantList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.TenantList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.TenantGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Tenant{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// connector produces an ExternalClient by extracting Grafana credentials from
// the referenced ProviderConfig.
// the referenced ProviderConfig.
type connector struct {
	kube   client.Client
	usage  *resource.ProviderConfigUsageTracker
	logger logging.Logger
}

// Connect extracts credentials from the ProviderConfig, creates a Grafana
// client, and returns an external client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Tenant)
	if !ok {
		return nil, errors.New(errNotTenant)
	}

	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	grafanaURL, creds, err := c.extractConfig(ctx, cr)
	if err != nil {
		return nil, err
	}

	gClient, err := grafana.NewClient(grafanaURL, creds)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{
		kube:   c.kube,
		sso:    gClient.SsoSettings,
		logger: c.logger,
	}, nil
}

// extractConfig reads the ProviderConfig (namespaced or cluster-scoped) and
// returns the Grafana URL and raw credential bytes.
func (c *connector) extractConfig(ctx context.Context, cr *v1alpha1.Tenant) (string, []byte, error) {
	ref := cr.Spec.ProviderConfigReference
	if ref == nil {
		return "", nil, errors.New(errGetPC + ": providerConfigRef is not set")
	}

	kind := ref.Kind
	if kind == "" || kind == apisv1alpha1.ProviderConfigKind {
		pc := &apisv1alpha1.ProviderConfig{}
		if err := c.kube.Get(ctx, client.ObjectKey{
			Namespace: cr.GetNamespace(),
			Name:      ref.Name,
		}, pc); err != nil {
			return "", nil, errors.Wrap(err, errGetPC)
		}
		data, err := resource.CommonCredentialExtractor(ctx, pc.Spec.Credentials.Source, c.kube, pc.Spec.Credentials.CommonCredentialSelectors)
		if err != nil {
			return "", nil, errors.Wrap(err, errGetCreds)
		}
		return pc.Spec.GrafanaURL, data, nil
	}

	if kind == apisv1alpha1.ClusterProviderConfigKind {
		pc := &apisv1alpha1.ClusterProviderConfig{}
		if err := c.kube.Get(ctx, client.ObjectKey{Name: ref.Name}, pc); err != nil {
			return "", nil, errors.Wrap(err, errGetPC)
		}
		data, err := resource.CommonCredentialExtractor(ctx, pc.Spec.Credentials.Source, c.kube, pc.Spec.Credentials.CommonCredentialSelectors)
		if err != nil {
			return "", nil, errors.Wrap(err, errGetCreds)
		}
		return pc.Spec.GrafanaURL, data, nil
	}

	return "", nil, errors.New(errGetPC + ": unsupported provider config kind: " + kind)
}

// external observes, creates, updates, and deletes Tenant resources,
// syncing org_mapping to Grafana SSO settings on each mutation.
// syncing org_mapping to Grafana SSO settings on each mutation.
type external struct {
	kube   client.Client
	sso    grafana.SSOClient
	logger logging.Logger
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Tenant)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotTenant)
	}

	// If the external name is not set, the resource has not been created yet.
	if meta.GetExternalName(cr) == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// For this "virtual" resource type where the CR is the source of truth,
	// if the resource is being deleted, sync to Grafana (remove tenant from
	// mapping) and then report ResourceExists: false so the managed reconciler
	// can remove the finalizer. This replaces the normal Delete flow.
	if cr.GetDeletionTimestamp() != nil {
		if err := c.syncGrafanaOrgMapping(ctx, cr, true); err != nil {
			c.logger.Info("Failed to sync Grafana org mapping during delete", "error", err)
		}
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Compare spec vs status to determine if an update is needed.
	// The status is synced to spec during Create/Update and persisted by the
	// managed reconciler.
	upToDate := isUpToDate(cr)

	// For virtual resources, explicitly set the Available condition when up-to-date.
	// This ensures the Ready status is properly reflected.
	if upToDate {
		cr.SetConditions(xpv1.Available())
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Tenant)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotTenant)
	}

	meta.SetExternalName(cr, cr.Spec.ForProvider.TenantID)
	syncStatus(cr)

	// Grafana sync is best-effort; log errors but don't block resource creation.
	// The CR itself is the source of truth for this resource type.
	if err := c.syncGrafanaOrgMapping(ctx, cr, false); err != nil {
		c.logger.Info("Failed to sync Grafana org mapping", "error", err)
	}

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Tenant)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotTenant)
	}

	syncStatus(cr)

	// Grafana sync is best-effort; log errors but don't block resource updates.
	// The CR itself is the source of truth for this resource type.
	if err := c.syncGrafanaOrgMapping(ctx, cr, false); err != nil {
		c.logger.Info("Failed to sync Grafana org mapping", "error", err)
	}

	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Tenant)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotTenant)
	}

	// Grafana sync is best-effort; log errors but don't block resource deletion.
	// The CR itself is the source of truth for this resource type.
	if err := c.syncGrafanaOrgMapping(ctx, cr, true); err != nil {
		c.logger.Info("Failed to sync Grafana org mapping during delete", "error", err)
	}

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

// syncGrafanaOrgMapping lists all Tenants, builds org_mapping, and writes it to
// Grafana SSO settings. If deleting is true, the current tenant is excluded.
func (c *external) syncGrafanaOrgMapping(ctx context.Context, cr *v1alpha1.Tenant, deleting bool) error {
	list := &v1alpha1.TenantList{}
	if err := c.kube.List(ctx, list); err != nil {
		return errors.Wrap(err, errListTenants)
	}

	mappings := make([]grafana.TenantMapping, 0, len(list.Items))
	for i := range list.Items {
		t := &list.Items[i]
		// Skip the tenant being deleted.
		if deleting && t.GetUID() == cr.GetUID() {
			continue
		}
		mappings = append(mappings, grafana.TenantMapping{
			OrgID:        t.Spec.ForProvider.OrgID,
			ViewerGroups: t.Spec.ForProvider.ViewerGroups,
			EditorGroups: t.Spec.ForProvider.EditorGroups,
		})
	}

	orgMapping := grafana.BuildOrgMapping(mappings)
	c.logger.Debug("Syncing Grafana org mapping", "org_mapping", orgMapping)

	if err := grafana.SyncOrgMapping(ctx, c.sso, mappings); err != nil {
		return errors.Wrap(err, "cannot sync Grafana org mapping")
	}
	return nil
}

// syncStatus copies spec fields into status and sets the lastUpdated timestamp.
func syncStatus(cr *v1alpha1.Tenant) {
	cr.Status.AtProvider = v1alpha1.TenantObservation{
		TenantID:     cr.Spec.ForProvider.TenantID,
		OrgID:        cr.Spec.ForProvider.OrgID,
		Admins:       cr.Spec.ForProvider.Admins,
		ViewerGroups: cr.Spec.ForProvider.ViewerGroups,
		EditorGroups: cr.Spec.ForProvider.EditorGroups,
		Retention:    cr.Spec.ForProvider.Retention,
		LastUpdated:  time.Now().UTC().Format(time.RFC3339),
	}
}

// isUpToDate compares spec.forProvider against status.atProvider.
func isUpToDate(cr *v1alpha1.Tenant) bool {
	spec := cr.Spec.ForProvider
	obs := cr.Status.AtProvider

	if spec.TenantID != obs.TenantID {
		return false
	}
	if spec.OrgID != obs.OrgID {
		return false
	}
	if spec.Retention != obs.Retention {
		return false
	}
	if !slicesEqual(spec.Admins, obs.Admins) {
		return false
	}
	if !slicesEqual(spec.ViewerGroups, obs.ViewerGroups) {
		return false
	}
	if !slicesEqual(spec.EditorGroups, obs.EditorGroups) {
		return false
	}
	return true
}

// slicesEqual compares two string slices, treating nil and empty as equivalent.
func slicesEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
