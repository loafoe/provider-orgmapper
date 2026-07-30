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
	"sort"
	"strings"
)

// ResolveOrgIDs returns the sorted, de-duplicated list of Grafana org
// identifiers (as they appear in orgMapping) that any of the given claim
// groups grants access to, based on Grafana's org_mapping semantics.
// Entries are of the form <group>:<orgId>:<role>, comma-separated, where the
// group field may use glob wildcards (* and ?) and colons within a field are
// escaped as \:. Role is ignored: this reports access, not role. Malformed
// entries are skipped rather than causing an error.
func ResolveOrgIDs(orgMapping string, claimGroups []string) []string {
	if orgMapping == "" || len(claimGroups) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var orgIDs []string

	for entry := range strings.SplitSeq(orgMapping, ",") {
		fields := splitUnescapedColon(strings.TrimSpace(entry))
		if len(fields) != 3 {
			continue
		}
		pattern, orgID := fields[0], fields[1]

		for _, g := range claimGroups {
			if !globMatch(pattern, g) {
				continue
			}
			if !seen[orgID] {
				seen[orgID] = true
				orgIDs = append(orgIDs, orgID)
			}
			break
		}
	}

	sort.Strings(orgIDs)
	return orgIDs
}

// globMatch reports whether s matches the glob pattern, where * matches any
// sequence of characters (including none) and ? matches exactly one
// character. Unlike filepath.Match, there is no special treatment of '/'.
func globMatch(pattern, s string) bool {
	return globMatchBytes([]byte(pattern), []byte(s))
}

func globMatchBytes(pattern, s []byte) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			// Trailing * matches the rest of s.
			if len(pattern) == 1 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if globMatchBytes(pattern[1:], s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pattern, s = pattern[1:], s[1:]
		default:
			if len(s) == 0 || pattern[0] != s[0] {
				return false
			}
			pattern, s = pattern[1:], s[1:]
		}
	}
	return len(s) == 0
}

// splitUnescapedColon splits s on ':' characters that are not preceded by a
// backslash, then unescapes \: back to : within each resulting field.
func splitUnescapedColon(s string) []string {
	var fields []string
	var cur strings.Builder

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == ':' {
			cur.WriteByte(':')
			i++
			continue
		}
		if s[i] == ':' {
			fields = append(fields, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(s[i])
	}
	fields = append(fields, cur.String())
	return fields
}
