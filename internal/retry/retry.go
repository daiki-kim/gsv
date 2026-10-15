// Package retry retries Google API calls on rate limiting and server errors.
package retry

import (
	"context"
	"errors"
	"time"

	"google.golang.org/api/googleapi"
)

// BaseDelay is the wait before the first retry; it doubles on each retry.
var BaseDelay = 500 * time.Millisecond

const MaxRetries = 3

// Do runs fn and retries it up to MaxRetries times while it fails with HTTP 429 or 5xx.
func Do(ctx context.Context, fn func() error) error {
	delay := BaseDelay
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil || attempt == MaxRetries || !retryable(err) {
			return err
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
		delay *= 2
	}
}

func retryable(err error) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return gerr.Code == 429 || gerr.Code >= 500
	}
	return false
}
