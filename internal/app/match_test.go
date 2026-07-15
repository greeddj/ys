package app

import "testing"

func TestBuildMatcher(t *testing.T) {
	type probe struct {
		dotted, last string
		want         bool
	}

	tests := []struct {
		name      string
		key       string
		probes    []probe
		regexMode bool
		pathMode  bool
	}{
		{
			name: "default matches last segment",
			key:  "dhcp",
			probes: []probe{
				{"secrets.crm.settings.dhcp", "dhcp", true},
				{"db.dhcp", "dhcp", true},
				{"a.dhcp_lease", "dhcp_lease", false},
				{"dhcp.enabled", "enabled", false},
			},
		},
		{
			name:     "path suffix",
			key:      "settings.dhcp",
			pathMode: true,
			probes: []probe{
				{"secrets.crm.settings.dhcp", "dhcp", true},
				{"settings.dhcp", "dhcp", true},
				{"other.dhcp", "dhcp", false},
				{"settings.dhcp.child", "child", false},
			},
		},
		{
			name:      "regexp over full path",
			key:       `settings\.(dhcp|dns)$`,
			regexMode: true,
			probes: []probe{
				{"a.settings.dhcp", "dhcp", true},
				{"a.settings.dns", "dns", true},
				{"a.settings.dhcpx", "dhcpx", false},
				{"settings.dhcp.z", "z", false},
			},
		},
		{
			name:      "regexp wins when both modes set",
			key:       "dhcp",
			regexMode: true,
			pathMode:  true,
			probes: []probe{
				// Regexp matches anywhere in the dotted path; the leaf is ignored.
				{"x.dhcp.y", "y", true},
				{"a.b", "b", false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := buildMatcher(tt.key, tt.regexMode, tt.pathMode)
			if err != nil {
				t.Fatalf("buildMatcher(%q): unexpected error: %v", tt.key, err)
			}
			for _, p := range tt.probes {
				if got := m(p.dotted, p.last); got != p.want {
					t.Errorf("match(%q, %q) = %v, want %v", p.dotted, p.last, got, p.want)
				}
			}
		})
	}
}

func TestBuildMatcherBadRegexp(t *testing.T) {
	if _, err := buildMatcher("(", true, false); err == nil {
		t.Fatal("expected an error for an invalid regexp, got nil")
	}
}
