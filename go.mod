module github.com/greeddj/ys

go 1.27.3

tool (
	golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment
	golang.org/x/vuln/cmd/govulncheck
	honnef.co/go/tools/cmd/staticcheck
)

require (
	github.com/urfave/cli/v3 v3.14.0
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	golang.org/x/exp/typeparams v0.0.0-20261014143641-f0251eb4aa3e // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/telemetry v0.0.0-20261021172232-6cd571d43fbb // indirect
	golang.org/x/tools v0.52.0 // indirect
	golang.org/x/vuln v1.9.0 // indirect
	honnef.co/go/tools v0.8.2 // indirect
)
