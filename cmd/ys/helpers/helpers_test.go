package helpers

import (
	"os"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	tests := []struct {
		name                           string
		version, commit, date, builtBy string
		wantContains                   []string
	}{
		{
			name:    "full metadata",
			version: "1.2.3", commit: "deadbee", date: "2020-01-01", builtBy: "just",
			wantContains: []string{"1.2.3", "commit deadbee", "built by just", "2020-01-01"},
		},
		{
			name:    "commit without date",
			version: "1.0", commit: "abc", date: "", builtBy: "x",
			wantContains: []string{"1.0", "commit abc", "built by x"},
		},
		{
			name:    "defaults when empty",
			version: "", commit: "", date: "", builtBy: "",
			wantContains: []string{"latest", "built by go"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Version(tt.version, tt.commit, tt.date, tt.builtBy)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Version() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestWantColor(t *testing.T) {
	tests := []struct {
		name           string
		force, disable bool
		noColor        bool
		want           bool
	}{
		{name: "disable wins over force", force: true, disable: true, want: false},
		{name: "disable off, force on", force: true, disable: false, want: true},
		{name: "explicit disable", force: false, disable: true, want: false},
		{name: "NO_COLOR disables in auto mode", noColor: true, want: false},
		{name: "force overrides NO_COLOR", force: true, noColor: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.noColor {
				t.Setenv("NO_COLOR", "1")
			} else {
				t.Setenv("NO_COLOR", "")
			}
			if got := WantColor(tt.force, tt.disable); got != tt.want {
				t.Errorf("WantColor(force=%v, disable=%v) = %v, want %v", tt.force, tt.disable, got, tt.want)
			}
		})
	}
}

func TestIsTerminalOnRegularFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()

	if isTerminal(f) {
		t.Error("isTerminal(regular file) = true, want false")
	}
}
