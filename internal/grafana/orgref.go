package grafana

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// organizationGVK identifies the provider-gf Organization managed resource.
// This is a soft contract on provider-gf's CRD, not a Go dependency on it.
var organizationGVK = schema.GroupVersionKind{
	Group:   "oss.gf.m.crossplane.io",
	Version: "v1alpha1",
	Kind:    "Organization",
}

// ResolveOrganizationID fetches a provider-gf Organization by name in the
// given namespace and returns its Grafana-assigned org ID from
// status.atProvider.id, as a string. Returns an error if the Organization
// doesn't exist yet, or exists but hasn't been assigned an ID yet -both
// are treated as retryable by callers via the normal reconcile error path.
func ResolveOrganizationID(ctx context.Context, kube client.Client, namespace, name string) (string, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(organizationGVK)

	if err := kube.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, u); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("organization %q not found in namespace %q: %w", name, namespace, err)
		}
		return "", fmt.Errorf("cannot get organization %q: %w", name, err)
	}

	val, found, err := unstructured.NestedFieldNoCopy(u.Object, "status", "atProvider", "id")
	if err != nil {
		return "", fmt.Errorf("organization %q has malformed status: %w", name, err)
	}
	if !found {
		return "", fmt.Errorf("organization %q has no status.atProvider.id yet", name)
	}

	var id int64
	switch v := val.(type) {
	case int64:
		id = v
	case float64:
		id = int64(v)
	default:
		return "", fmt.Errorf("organization %q status.atProvider.id has unexpected type %T", name, val)
	}
	if id == 0 {
		return "", fmt.Errorf("organization %q has not been assigned an id yet", name)
	}

	return fmt.Sprintf("%d", id), nil
}
