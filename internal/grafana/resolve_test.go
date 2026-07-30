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
	"reflect"
	"testing"
)

func TestResolveOrgIDs(t *testing.T) {
	cases := map[string]struct {
		orgMapping  string
		claimGroups []string
		want        []string
	}{
		"EmptyMapping": {
			orgMapping:  "",
			claimGroups: []string{"team-a"},
			want:        nil,
		},
		"EmptyClaimGroups": {
			orgMapping:  "team-a:org-1:Viewer",
			claimGroups: nil,
			want:        nil,
		},
		"ExactMatch": {
			orgMapping:  "team-a:org-1:Viewer",
			claimGroups: []string{"team-a"},
			want:        []string{"org-1"},
		},
		"NoMatch": {
			orgMapping:  "team-a:org-1:Viewer",
			claimGroups: []string{"team-b"},
			want:        nil,
		},
		"WildcardStarMatchesAny": {
			orgMapping:  "*:org-1:Viewer",
			claimGroups: []string{"anything"},
			want:        []string{"org-1"},
		},
		"WildcardPrefixMatch": {
			orgMapping:  "team-*:org-1:Viewer",
			claimGroups: []string{"team-eng"},
			want:        []string{"org-1"},
		},
		"WildcardQuestionMark": {
			orgMapping:  "team-?:org-1:Viewer",
			claimGroups: []string{"team-a"},
			want:        []string{"org-1"},
		},
		"MultipleGroupsAcrossOrgs": {
			orgMapping:  "team-a:org-1:Viewer,team-b:org-2:Editor",
			claimGroups: []string{"team-a", "team-b"},
			want:        []string{"org-1", "org-2"},
		},
		"OneGroupMatchesMultipleOrgs": {
			orgMapping:  "team-a:org-1:Viewer,team-a:org-2:Editor",
			claimGroups: []string{"team-a"},
			want:        []string{"org-1", "org-2"},
		},
		"DuplicateOrgsAreDeduplicated": {
			orgMapping:  "team-a:org-1:Viewer,team-b:org-1:Editor",
			claimGroups: []string{"team-a", "team-b"},
			want:        []string{"org-1"},
		},
		"ResultIsSorted": {
			orgMapping:  "team-a:org-2:Viewer,team-a:org-1:Editor",
			claimGroups: []string{"team-a"},
			want:        []string{"org-1", "org-2"},
		},
		"EscapedColonInGroupName": {
			orgMapping:  `oidc\:team\:viewers:org-1:Viewer`,
			claimGroups: []string{"oidc:team:viewers"},
			want:        []string{"org-1"},
		},
		"MalformedEntrySkipped": {
			orgMapping:  "team-a:org-1,team-b:org-2:Viewer",
			claimGroups: []string{"team-a", "team-b"},
			want:        []string{"org-2"},
		},
		"CaseSensitiveNoMatch": {
			orgMapping:  "Team-A:org-1:Viewer",
			claimGroups: []string{"team-a"},
			want:        nil,
		},
		"WildcardMatchesAcrossSlash": {
			orgMapping:  "ou/*:org-1:Viewer",
			claimGroups: []string{"ou/team/a"},
			want:        []string{"org-1"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := ResolveOrgIDs(tc.orgMapping, tc.claimGroups)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ResolveOrgIDs(%q, %v) = %v, want %v", tc.orgMapping, tc.claimGroups, got, tc.want)
			}
		})
	}
}
