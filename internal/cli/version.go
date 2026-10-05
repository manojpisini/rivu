package cli

import (
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// buildInfo resolves version metadata. Defaults ("dev"/"none"/"unknown")
// fall back to what the toolchain embedded for `go install` builds:
// module version plus the VCS revision and time (X-05).
func buildInfo(version, commit, date string) (string, string, string) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return version, commit, date
	}
	if version == "dev" || version == "" {
		if bi.Main.Version != "" {
			version = bi.Main.Version
		}
	}
	for _, s := range bi.Settings {
		switch {
		case s.Key == "vcs.revision" && (commit == "none" || commit == "dev" || commit == "") && len(s.Value) >= 7:
			if len(s.Value) > 12 {
				s.Value = s.Value[:12]
			}
			commit = s.Value
		case s.Key == "vcs.time" && (date == "unknown" || date == "") && s.Value != "":
			date = s.Value
		}
	}
	return version, commit, date
}

// versionOutput is the `rivu version --json` shape; schema is part of
// the scripting contract (documented in docs/cli.md).
type versionOutput struct {
	Schema  int    `json:"schema"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func versionCmd(version, commit, date string) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{Use: "version", Short: "Print version information", RunE: func(cmd *cobra.Command, _ []string) error {
		if jsonOut {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(versionOutput{
				Schema: 1, Version: version, Commit: commit, Date: date,
				Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH,
			})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s (%s, %s)\n", version, commit, date)
		return nil
	}}
	c.Flags().BoolVar(&jsonOut, "json", false, "Output build info as JSON (schema 1)")
	return c
}
