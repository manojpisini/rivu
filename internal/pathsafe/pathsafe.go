// Package pathsafe checks that a destination path stays inside an allowed
// base directory. Every mutating command routes its destination through
// Contained before writing (spec §4.4 rule 4).
package pathsafe

import (
	"path/filepath"
	"strings"
)

// Contained reports whether target is base itself or lies under base.
// Both paths are made absolute and cleaned first, so relative segments
// (a/../b) and symlinks in the textual path cannot escape. On
// case-insensitive filesystems an exact-case prefix is used, which only
// ever rejects same-dir-different-case paths, never accepts an escape.
func Contained(base, target string) bool {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	absBase = filepath.Clean(absBase)
	absTarget = filepath.Clean(absTarget)
	if absTarget == absBase {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(absTarget, absBase+sep)
}
