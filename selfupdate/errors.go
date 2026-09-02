package selfupdate

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

var (
	// ErrUpdateAvailable is returned from Run in check mode when the selected
	// target is a different actionable release. ExitCode maps it to 10.
	ErrUpdateAvailable = errors.New("selfupdate: update available")
	// ErrConfirmationRequired is returned when apply needs a TTY or --yes.
	ErrConfirmationRequired = errors.New("selfupdate: confirmation required")
	// ErrConcurrentUpdate is returned when another update holds the target
	// lock or an overlapping Run is already in progress on the same Updater.
	ErrConcurrentUpdate = errors.New("selfupdate: concurrent update")
	// ErrManagedInstall is returned when managed stop, reconcile, start, or
	// health fails. Recovery errors are joined with it.
	ErrManagedInstall = errors.New("selfupdate: managed install failed")
	// ErrUnsupportedPlatform is returned when the requested platform is not
	// in the selector's caller-supplied matrix.
	ErrUnsupportedPlatform = errors.New("selfupdate: unsupported platform")
	// ErrIntegrity is returned when size, digest, or checksum verification
	// fails.
	ErrIntegrity = errors.New("selfupdate: integrity check failed")
	// ErrMutableRelease is returned when selected release metadata is not
	// immutable.
	ErrMutableRelease = errors.New("selfupdate: release is not immutable")
	// ErrRateLimited is the sentinel unwrapped by RateLimitError.
	ErrRateLimited = errors.New("selfupdate: rate limited")
)

var (
	errForceRequired = errors.New("selfupdate: replacing a local build requires --force")
	errLatestOlder   = errors.New("selfupdate: latest release is older than the running version")
)

// RateLimitError retains GitHub rate-limit guidance. Missing or malformed
// optional headers yield zero values rather than guessed guidance.
type RateLimitError struct {
	// StatusCode is the HTTP status, typically 429 or 403.
	StatusCode int
	// RetryAfter is parsed from Retry-After when present.
	RetryAfter time.Duration
	// Reset is parsed from X-RateLimit-Reset when present.
	Reset time.Time
	// Remaining is parsed from X-RateLimit-Remaining when present.
	Remaining int64
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	if e == nil {
		return ErrRateLimited.Error()
	}
	return fmt.Sprintf("selfupdate: rate limited: status %d remaining %d retry-after %s reset %s",
		e.StatusCode, e.Remaining, e.RetryAfter, e.Reset.UTC().Format(time.RFC3339))
}

// Unwrap reports ErrRateLimited.
func (e *RateLimitError) Unwrap() error {
	return ErrRateLimited
}

// ExitCode maps a Run outcome to the canonical process status. It returns 10
// only when ErrUpdateAvailable is in the error chain, 1 for any other error,
// and 0 otherwise. The library never calls os.Exit.
func ExitCode(result Result, err error) int {
	_ = result
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrUpdateAvailable) {
		return 10
	}
	return 1
}

func itoa(v uint64) string {
	return strconv.FormatUint(v, 10)
}
