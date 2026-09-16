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

	t.Run("id is negative", func(t *testing.T) {
		kube := clfake.NewClientBuilder().WithScheme(newOrgScheme()).WithObjects(newOrg("ns1", "acme-org", int64(-1))).Build()
		_, err := ResolveOrganizationID(ctx, kube, "ns1", "acme-org")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("malformed status shape", func(t *testing.T) {
		org := &unstructured.Unstructured{}
		org.SetGroupVersionKind(organizationGVK)
		org.SetNamespace("ns1")
		org.SetName("acme-org")
		// status.atProvider is a scalar rather than a map, so traversing to
		// status.atProvider.id fails with an accessor error rather than a
		// simple "not found".
		_ = unstructured.SetNestedField(org.Object, "not-a-map", "status", "atProvider")
		kube := clfake.NewClientBuilder().WithScheme(newOrgScheme()).WithObjects(org).Build()
		_, err := ResolveOrganizationID(ctx, kube, "ns1", "acme-org")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("id has unexpected type", func(t *testing.T) {
		kube := clfake.NewClientBuilder().WithScheme(newOrgScheme()).WithObjects(newOrg("ns1", "acme-org", "42")).Build()
		_, err := ResolveOrganizationID(ctx, kube, "ns1", "acme-org")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
