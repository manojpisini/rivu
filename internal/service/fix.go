package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/manojpisini/rivu/internal/doctor"
	"github.com/manojpisini/rivu/internal/registry"
)

// doctorFixIDs are the safe `rivu doctor --fix` remedies (P6.03):
// create-if-missing files and machine-owned regeneration only — never
// a project folder (safety rule 1).
var doctorFixIDs = map[string]string{
	"readme": "create README.md",
	"map":    "rebuild Bank and Map",
}

// DoctorFixPlan lists what `--fix` would do for these reports, as
// "slug: action" lines — the Plan half of Plan → confirm → Apply.
func DoctorFixPlan(rs []doctor.Report, ids []string) []string {
	want := fixIDSet(ids)
	var lines []string
	for _, r := range rs {
		for _, c := range r.Checks {
			if !c.OK && c.Fixable != "" && want[c.Fixable] {
				line := r.Project.Slug + ": " + doctorFixIDs[c.Fixable]
				if !containsLine(lines, line) {
					lines = append(lines, line)
				}
			}
		}
	}
	return lines
}

// ApplyDoctorFixes performs the safe fixes named by ids on every
// report that needs them and returns how many were applied. It only
// creates what is missing: an existing README is never rewritten, and
// `map` rebuilds the machine-owned Bank/Map files in place.
func (a *App) ApplyDoctorFixes(rs []doctor.Report, ids []string) (int, error) {
	want := fixIDSet(ids)
	for id := range want {
		if _, ok := doctorFixIDs[id]; !ok {
			return 0, fmt.Errorf("unknown fix id %q (want readme or map)", id)
		}
	}
	applied := 0
	var errs []error
	for _, r := range rs {
		perProject := map[string]bool{}
		for _, c := range r.Checks {
			if !c.OK && c.Fixable != "" && want[c.Fixable] {
				perProject[c.Fixable] = true
			}
		}
		for id := range perProject {
			var err error
			switch id {
			case "readme":
				err = writeReadmeIfMissing(r.Project)
			case "map":
				err = a.Map(r.Project.Slug)
			default:
				continue
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s %s: %w", r.Project.Slug, id, err))
				continue
			}
			applied++
		}
	}
	return applied, errors.Join(errs...)
}

// writeReadmeIfMissing creates a starter README (spec's protected
// create-if-missing list) and leaves any existing README/README.md
// untouched.
func writeReadmeIfMissing(p registry.Project) error {
	md := filepath.Join(p.Path, "README.md")
	if _, err := os.Stat(md); err == nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(p.Path, "README")); err == nil {
		return nil
	}
	body := "# " + p.Name + "\n\nDescribe this project here.\n"
	if err := os.WriteFile(md, []byte(body), 0644); err != nil {
		return fmt.Errorf("write README.md: %w", err)
	}
	return nil
}

func fixIDSet(ids []string) map[string]bool {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	return want
}

func containsLine(lines []string, s string) bool {
	for _, l := range lines {
		if l == s {
			return true
		}
	}
	return false
}
