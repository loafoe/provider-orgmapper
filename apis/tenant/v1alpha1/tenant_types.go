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

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
)

// TenantParameters are the configurable fields of a Tenant.
type TenantParameters struct {
	// TenantID is the unique identifier for this tenant.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	TenantID string `json:"tenantId"`

	// OrgID is the mapped organization identifier.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	OrgID string `json:"orgId"`

	// Admins is a list of tenant administrators (typically GitHub IDs).
	// +optional
	Admins []string `json:"admins,omitempty"`

	// ViewerGroups is a list of group claims that grant Viewer role in this tenant's Grafana org.
	// +optional
	ViewerGroups []string `json:"viewerGroups,omitempty"`

	// EditorGroups is a list of group claims that grant Editor role in this tenant's Grafana org.
	// +optional
	EditorGroups []string `json:"editorGroups,omitempty"`

	// Retention defines data retention settings for each signal type.
	// +kubebuilder:validation:Required
	Retention RetentionPolicy `json:"retention"`
}

// RetentionPolicy defines data retention durations for each signal type.
type RetentionPolicy struct {
	// Logs retention duration (e.g. "30d", "24h", "1w").
	// +kubebuilder:validation:Pattern=`^[0-9]+(d|h|w|m|y)$`
	// +optional
	Logs string `json:"logs,omitempty"`

	// Metrics retention duration.
	// +kubebuilder:validation:Pattern=`^[0-9]+(d|h|w|m|y)$`
	// +optional
	Metrics string `json:"metrics,omitempty"`

	// Traces retention duration.
	// +kubebuilder:validation:Pattern=`^[0-9]+(d|h|w|m|y)$`
	// +optional
	Traces string `json:"traces,omitempty"`

	// Profiles retention duration.
	// +kubebuilder:validation:Pattern=`^[0-9]+(d|h|w|m|y)$`
	// +optional
	Profiles string `json:"profiles,omitempty"`
}

// TenantObservation are the observable fields of a Tenant.
type TenantObservation struct {
	TenantID     string          `json:"tenantId,omitempty"`
	OrgID        string          `json:"orgId,omitempty"`
	Admins       []string        `json:"admins,omitempty"`
	ViewerGroups []string        `json:"viewerGroups,omitempty"`
	EditorGroups []string        `json:"editorGroups,omitempty"`
	Retention    RetentionPolicy `json:"retention,omitempty"`
	LastUpdated  string          `json:"lastUpdated,omitempty"`
}

// A TenantSpec defines the desired state of a Tenant.
type TenantSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              TenantParameters `json:"forProvider"`
}

// A TenantStatus represents the observed state of a Tenant.
type TenantStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          TenantObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="TENANT-ID",type="string",JSONPath=".spec.forProvider.tenantId"
// +kubebuilder:printcolumn:name="ORG-ID",type="string",JSONPath=".spec.forProvider.orgId"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,orgmapper}

// A Tenant is a managed resource that represents a tenant in the LGTM stack registry.
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantSpec   `json:"spec"`
	Status TenantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TenantList contains a list of Tenant
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

// Tenant type metadata.
var (
	TenantKind             = reflect.TypeOf(Tenant{}).Name()
	TenantGroupKind        = schema.GroupKind{Group: Group, Kind: TenantKind}.String()
	TenantKindAPIVersion   = TenantKind + "." + SchemeGroupVersion.String()
	TenantGroupVersionKind = SchemeGroupVersion.WithKind(TenantKind)
)

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}
