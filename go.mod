module github.com/greeddj/ys

go 1.27.1

tool (
	golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment
	golang.org/x/vuln/cmd/govulncheck
	honnef.co/go/tools/cmd/staticcheck
)

require (
	github.com/urfave/cli/v3 v3.12.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	golang.org/x/exp/typeparams v0.0.0-20260908205506-85c1c2202aba // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/telemetry v0.0.0-20260910141331-15ceca2b0a1f // indirect
	golang.org/x/tools v0.50.0 // indirect
	golang.org/x/vuln v1.8.0 // indirect
	honnef.co/go/tools v0.8.1 // indirect
)
