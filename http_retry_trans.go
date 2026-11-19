package thttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultRetryMaxCount is the default upper bound on total attempts, including the first request.
	DefaultRetryMaxCount = 3
	// DefaultRetryMinWaitTime is the default minimum backoff interval.
	DefaultRetryMinWaitTime = time.Duration(100) * time.Millisecond
	// DefaultRetryMaxWaitTime is the default maximum backoff interval.
	DefaultRetryMaxWaitTime = time.Duration(2000) * time.Millisecond
)

// CheckRetryFunc determines whether a failed or unsatisfactory round trip should be retried.
type CheckRetryFunc func(ctx context.Context, response *http.Response, err error) (bool, error)

// BackoffFunc computes the wait duration before the next retry attempt.
type BackoffFunc func(minWaitTime, maxWaitTime time.Duration, attemptNum int, response *http.Response) time.Duration

// RetryErrorFunc optionally transforms the response or error between retry attempts.
// When returning a different non-nil body, the hook must close the original or
// return a wrapper that closes it. The transport closes a body the hook drops.
type RetryErrorFunc func(response *http.Response, err error, retryNum int) (*http.Response, error)

// RetryTransOption configures the retrying [http.RoundTripper] used when [OptTransRetry] is set.
type RetryTransOption struct {
	retryMaxCount    int
	retryMinWaitTime time.Duration
	retryMaxWaitTime time.Duration

	checkRetryFunc CheckRetryFunc

	// backoff specifies the policy for how long to wait between retries
	backoffFunc BackoffFunc

	// retryErrorHandler specifies the custom error handler to use, if any
	retryErrorFunc RetryErrorFunc
}

// NewRetryTransOption returns a configuration using [DefaultRetryPolicy], [DefaultBackoff], and default limits.
func NewRetryTransOption() *RetryTransOption {
	return &RetryTransOption{
		retryMaxCount:    DefaultRetryMaxCount,
		retryMinWaitTime: DefaultRetryMinWaitTime,
		retryMaxWaitTime: DefaultRetryMaxWaitTime,

		checkRetryFunc: DefaultRetryPolicy,
		backoffFunc:    DefaultBackoff,
	}
}

// WithMaxCount sets the maximum number of total attempts, including the first request.
func (options *RetryTransOption) WithMaxCount(maxCount int) *RetryTransOption {
	options.retryMaxCount = maxCount

	return options
}

// WithWaitTime sets a nonnegative backoff range when maxWaitTime is at least minWaitTime.
func (options *RetryTransOption) WithWaitTime(minWaitTime, maxWaitTime time.Duration) *RetryTransOption {
	if minWaitTime >= 0 && maxWaitTime >= minWaitTime {
		options.retryMinWaitTime = minWaitTime
		options.retryMaxWaitTime = maxWaitTime
	}

	return options
}

// WithCheckRetry replaces the policy used to decide whether to retry. Nil uses [DefaultRetryPolicy].
func (options *RetryTransOption) WithCheckRetry(checkRetryFunc CheckRetryFunc) *RetryTransOption {
	options.checkRetryFunc = checkRetryFunc

	return options
}

// WithBackoff replaces the backoff computation between attempts. Nil uses [DefaultBackoff].
func (options *RetryTransOption) WithBackoff(backoffFunc BackoffFunc) *RetryTransOption {
	options.backoffFunc = backoffFunc

	return options
}

// WithRetryError sets an optional hook invoked to adjust the response or error before re-evaluating retry policy.
func (options *RetryTransOption) WithRetryError(retryErrorFunc RetryErrorFunc) *RetryTransOption {
	options.retryErrorFunc = retryErrorFunc

	return options
}

// DefaultRetryTransOption is a package-level default configuration mirroring [NewRetryTransOption].
var DefaultRetryTransOption = &RetryTransOption{
	retryMaxCount:    DefaultRetryMaxCount,
	retryMinWaitTime: DefaultRetryMinWaitTime,
	retryMaxWaitTime: DefaultRetryMaxWaitTime,

	checkRetryFunc: DefaultRetryPolicy,
	backoffFunc:    DefaultBackoff,
}

// retryTransport retries eligible methods whose bodies can be reopened independently.
type retryTransport struct {
	transport http.RoundTripper

	retryTransOption *RetryTransOption
}

// RoundTrip leaves each attempt's request and body with its delegate. A retry
// uses a fresh request so the delegate can finish reading the previous attempt.
func (transport *retryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx := request.Context()
	option := transport.retryTransOption
	hasBody := request.Body != nil && request.Body != http.NoBody
	canRetry := isRetryableMethod(request) && (!hasBody || request.GetBody != nil)
	attemptRequest := request

	for attempt := 0; ; attempt++ {
		response, err := transport.transport.RoundTrip(attemptRequest)
		retry, policyErr := option.checkRetryFunc(ctx, response, err)
		if option.retryErrorFunc != nil && (err != nil || retry || policyErr != nil) {
			var originalBody io.ReadCloser
			if response != nil {
				originalBody = response.Body
			}

			response, err = option.retryErrorFunc(response, err, attempt)
			// A replacement body can wrap the original, so only close a dropped body here.
			if originalBody != nil && (response == nil || response.Body == nil) {
				_ = originalBody.Close()
			}

			retry, policyErr = option.checkRetryFunc(ctx, response, err)
		}

		if !retry || !canRetry || attempt+1 >= option.retryMaxCount {
			if err := errors.Join(err, policyErr); err != nil {
				closeResponseBody(response)

				return nil, err
			}

			if response == nil {
				return nil, errors.New("thttp: retry transport returned no response or error")
			}

			return response, nil
		}

		waitTime := option.backoffFunc(option.retryMinWaitTime, option.retryMaxWaitTime, attempt, response)
		closeResponseBody(response)
		timer := time.NewTimer(waitTime)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-request.Cancel:
			timer.Stop()
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			return nil, context.Canceled
		case <-timer.C:
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		attemptRequest = request.Clone(ctx)
		if hasBody {
			body, err := request.GetBody()
			if err != nil {
				if body != nil {
					_ = body.Close()
				}

				return nil, fmt.Errorf("thttp: reopen request body: %w", err)
			}

			if body == nil {
				return nil, errors.New("thttp: reopened request body must not be nil")
			}

			attemptRequest.Body = body
		}
	}
}

// isRetryableMethod permits standard idempotent methods and explicit idempotency keys.
func isRetryableMethod(request *http.Request) bool {
	switch request.Method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions, http.MethodTrace:
		return true
	}

	return strings.TrimSpace(request.Header.Get("Idempotency-Key")) != "" ||
		strings.TrimSpace(request.Header.Get("X-Idempotency-Key")) != ""
}

// closeResponseBody releases discarded responses without waiting to drain an arbitrary body.
func closeResponseBody(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

// wrapRetryTransport returns a retrying decorator around transport, or [http.DefaultTransport] when transport is nil.
func wrapRetryTransport(transport http.RoundTripper, retryTransOption *RetryTransOption) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}

	if retryTransOption == nil {
		retryTransOption = DefaultRetryTransOption
	}

	if retryTransOption == nil {
		retryTransOption = NewRetryTransOption()
	}

	option := *retryTransOption
	if option.checkRetryFunc == nil {
		option.checkRetryFunc = DefaultRetryPolicy
	}

	if option.backoffFunc == nil {
		option.backoffFunc = DefaultBackoff
	}

	retryTransport := &retryTransport{
		transport:        transport,
		retryTransOption: &option,
	}

	return retryTransport
}
