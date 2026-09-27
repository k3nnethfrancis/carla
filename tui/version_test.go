package main

import "testing"

func TestVersionIdentifiesReleaseAndDevelopmentBuilds(t *testing.T) {
	oldVersion, oldRevision := appVersion, appRevision
	defer func() { appVersion, appRevision = oldVersion, oldRevision }()
	for _, tc := range []struct{ version, revision, want string }{
		{"0.1.0", "", "carla 0.1.0"},
		{"0.1.0", "abc123-dirty", "carla 0.1.0 (abc123-dirty)"},
		{"dev", "", "carla dev"},
	} {
		appVersion, appRevision = tc.version, tc.revision
		if got := versionString(); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}
