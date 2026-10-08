package cli

import (
	"errors"
	"fmt"

	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
)

var (
	// ErrNeedsConfirm marks a mutating command that ran without --yes;
	// exit code 4 (X-03).
	ErrNeedsConfirm = errors.New("confirmation required")
	// ErrWarnings marks a command that succeeded with warnings; exit
	// code 5 (X-03).
	ErrWarnings = errors.New("completed with warnings")
)

// ExitCode maps a command result to the process exit code (X-03):
// 0 ok, 1 error, 2 usage, 3 not found, 4 needs --yes, 5 warnings.
// ran reports whether command execution got past argument and flag
// validation; anything failing before that is a usage error.
func ExitCode(err error, ran bool) int {
	switch {
	case err == nil:
		return 0
	case !ran:
		return 2
	case errors.Is(err, registry.ErrNotFound), errors.Is(err, registry.ErrAmbiguous):
		return 3
	case errors.Is(err, registry.ErrConfluenceNotFound):
		return 3
	case errors.Is(err, config.ErrNotFound):
		return 3
	case errors.Is(err, ErrNeedsConfirm):
		return 4
	case errors.Is(err, ErrWarnings):
		return 5
	default:
		return 1
	}
}

// bulkFail classifies failures from a bulk operation (FlowBulk,
// MapBulk): missing/ambiguous queries become exit-3 errors joined at
// the end, any other failure wins first as exit 1. Empty queries are
// labelled (current).
func bulkFail(fs []service.Failure) error {
	var missing, other []error
	for _, f := range fs {
		label := f.Query
		if label == "" {
			label = "(current)"
		}
		wrapped := fmt.Errorf("%s: %w", label, f.Err)
		if errors.Is(f.Err, registry.ErrNotFound) || errors.Is(f.Err, registry.ErrAmbiguous) {
			missing = append(missing, wrapped)
		} else {
			other = append(other, wrapped)
		}
	}
	if len(other) > 0 {
		return errors.Join(other...)
	}
	return errors.Join(missing...)
}
