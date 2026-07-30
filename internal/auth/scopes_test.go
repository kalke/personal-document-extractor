package auth

import (
	"reflect"
	"testing"
)

func TestScopesFromClaims(t *testing.T) {
	cases := []struct {
		name        string
		permissions []string
		scope       string
		want        []string
	}{
		{
			name:        "permissions_win",
			permissions: []string{ScopeExtractWrite, ScopeAdmin},
			scope:       "openid profile",
			want:        []string{ScopeExtractWrite, ScopeAdmin},
		},
		{
			name:  "scope_fields",
			scope: "extract:write keys:manage openid",
			want:  []string{ScopeExtractWrite, ScopeKeysManage, "openid"},
		},
		{
			name: "defaults_when_empty",
			want: []string{ScopeExtractWrite, ScopeKeysManage},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scopesFromClaims(tc.permissions, tc.scope)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}
