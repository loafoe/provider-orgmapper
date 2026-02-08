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
	"testing"
)

func TestNewClient(t *testing.T) {
	cases := map[string]struct {
		url     string
		creds   []byte
		wantErr bool
	}{
		"TokenAuth": {
			url:   "https://grafana.example.com",
			creds: []byte("glsa_xxxxxxxxxxxx"),
		},
		"BasicAuth": {
			url:   "https://grafana.example.com",
			creds: []byte(`{"username":"admin","password":"secret"}`),
		},
		"URLWithPath": {
			url:   "https://grafana.example.com/grafana",
			creds: []byte("glsa_xxxxxxxxxxxx"),
		},
		"URLWithAPIPath": {
			url:   "https://grafana.example.com/api",
			creds: []byte("glsa_xxxxxxxxxxxx"),
		},
		"InvalidURL": {
			url:     "://bad",
			creds:   []byte("token"),
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c, err := NewClient(tc.url, tc.creds)
			if tc.wantErr {
				if err == nil {
					t.Error("NewClient(...): expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NewClient(...): unexpected error: %v", err)
				return
			}
			if c == nil {
				t.Error("NewClient(...): expected non-nil client")
			}
		})
	}
}

func TestBasePath(t *testing.T) {
	cases := map[string]struct {
		path string
		want string
	}{
		"Empty":         {path: "", want: "/api"},
		"SlashOnly":     {path: "/", want: "/api"},
		"CustomPath":    {path: "/grafana", want: "/grafana/api"},
		"AlreadyHasAPI": {path: "/api", want: "/api"},
		"TrailingSlash": {path: "/grafana/", want: "/grafana/api"},
		"NestedWithAPI": {path: "/prefix/api", want: "/prefix/api"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := basePath(tc.path)
			if got != tc.want {
				t.Errorf("basePath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
