// internal/grafana/orgref_test.go
package grafana

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newOrgScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(organizationGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(organizationGVK.GroupVersion().WithKind("OrganizationList"), &unstructured.UnstructuredList{})
	return scheme
}

func newOrg(namespace, name string, id any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(organizationGVK)
	u.SetNamespace(namespace)
	u.SetName(name)
	if id != nil {
		_ = unstructured.SetNestedField(u.Object, id, "status", "atProvider", "id")
	}
	return u
}

func TestResolveOrganizationID(t *testing.T) {
	ctx := context.Background()

	t.Run("ready with int64 id", func(t *testing.T) {
		kube := clfake.NewClientBuilder().WithScheme(newOrgScheme()).WithObjects(newOrg("ns1", "acme-org", int64(42))).Build()
		got, err := ResolveOrganizationID(ctx, kube, "ns1", "acme-org")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "42" {
			t.Errorf("got %q, want %q", got, "42")
		}
	})

	t.Run("ready with float64 id", func(t *testing.T) {
		kube := clfake.NewClientBuilder().WithScheme(newOrgScheme()).WithObjects(newOrg("ns1", "acme-org", float64(7))).Build()
		got, err := ResolveOrganizationID(ctx, kube, "ns1", "acme-org")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "7" {
			t.Errorf("got %q, want %q", got, "7")
		}
	})

	t.Run("not found", func(t *testing.T) {
		kube := clfake.NewClientBuilder().WithScheme(newOrgScheme()).Build()
		_, err := ResolveOrganizationID(ctx, kube, "ns1", "missing-org")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("found but not ready", func(t *testing.T) {
		kube := clfake.NewClientBuilder().WithScheme(newOrgScheme()).WithObjects(newOrg("ns1", "acme-org", nil)).Build()
		_, err := ResolveOrganizationID(ctx, kube, "ns1", "acme-org")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("id is zero", func(t *testing.T) {
		kube := clfake.NewClientBuilder().WithScheme(newOrgScheme()).WithObjects(newOrg("ns1", "acme-org", int64(0))).Build()
		_, err := ResolveOrganizationID(ctx, kube, "ns1", "acme-org")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
