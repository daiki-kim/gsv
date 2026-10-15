package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
)

func init() { BaseDelay = time.Millisecond }

func codeErr(code int) error { return &googleapi.Error{Code: code} }

func TestRetriesThenSucceeds(t *testing.T) {
	calls := 0
	err := Do(context.Background(), func() error {
		calls++
		if calls < 3 {
			return codeErr(429)
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err = %v, calls = %d", err, calls)
	}
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	calls := 0
	err := Do(context.Background(), func() error { calls++; return codeErr(503) })
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr.Code != 503 || calls != MaxRetries+1 {
		t.Fatalf("err = %v, calls = %d", err, calls)
	}
}

func TestDoesNotRetryClientErrors(t *testing.T) {
	calls := 0
	_ = Do(context.Background(), func() error { calls++; return codeErr(404) })
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestDoesNotRetryPlainErrors(t *testing.T) {
	calls := 0
	_ = Do(context.Background(), func() error { calls++; return errors.New("boom") })
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Do(ctx, func() error { return codeErr(429) })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
