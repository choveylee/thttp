package thttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
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
type CheckRetryFunc func(ctx context.Context, resp *http.Response, err error) (bool, error)

// BackoffFunc computes the wait duration before the next retry attempt.
type BackoffFunc func(minWaitTime, maxWaitTime time.Duration, attemptNum int, resp *http.Response) time.Duration

// RetryErrorFunc optionally transforms the response or error between retry attempts.
type RetryErrorFunc func(resp *http.Response, err error, retryNum int) (*http.Response, error)

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
func (p *RetryTransOption) WithMaxCount(maxCount int) *RetryTransOption {
	p.retryMaxCount = maxCount

	return p
}

// WithWaitTime sets the backoff range; values are applied only when maxWaitTime is not less than minWaitTime.
func (p *RetryTransOption) WithWaitTime(minWaitTime, maxWaitTime time.Duration) *RetryTransOption {
	if maxWaitTime >= minWaitTime {
		p.retryMinWaitTime = minWaitTime
		p.retryMaxWaitTime = maxWaitTime
	}

	return p
}

// WithCheckRetry replaces the policy used to decide whether to retry.
func (p *RetryTransOption) WithCheckRetry(checkRetryFunc CheckRetryFunc) *RetryTransOption {
	p.checkRetryFunc = checkRetryFunc

	return p
}

// WithBackoff replaces the backoff computation between attempts.
func (p *RetryTransOption) WithBackoff(backoffFunc BackoffFunc) *RetryTransOption {
	p.backoffFunc = backoffFunc

	return p
}

// WithRetryError sets an optional hook invoked to adjust the response or error before re-evaluating retry policy.
func (p *RetryTransOption) WithRetryError(retryErrorFunc RetryErrorFunc) *RetryTransOption {
	p.retryErrorFunc = retryErrorFunc

	return p
}

// DefaultRetryTransOption is a package-level default configuration mirroring [NewRetryTransOption].
var DefaultRetryTransOption = &RetryTransOption{
	retryMaxCount:    DefaultRetryMaxCount,
	retryMinWaitTime: DefaultRetryMinWaitTime,
	retryMaxWaitTime: DefaultRetryMaxWaitTime,

	checkRetryFunc: DefaultRetryPolicy,
	backoffFunc:    DefaultBackoff,
}

// retryTransport replays request bodies and applies retry and backoff policies around a delegate [http.RoundTripper].
// When [http.Request.GetBody] is set, each attempt (including the first) obtains a fresh [http.Request.Body] via GetBody
// so large or streaming payloads are not buffered in memory. Otherwise the body is read once with [io.ReadAll] and replayed from a buffer.
type retryTransport struct {
	transport http.RoundTripper

	retryTransOption *RetryTransOption
}

// RoundTrip implements [http.RoundTripper].
func (p *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var response *http.Response
	var respErr, checkErr error
	var retry bool

	attemptNum := 0

	retryTransOption := p.retryTransOption

	reuseBody := false

	if req.GetBody != nil {
		reuseBody = true
	}

	var body []byte

	if !reuseBody && req.Body != nil {
		var err error

		body, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
	}

	for i := 0; ; i++ {
		attemptNum++

		if reuseBody {
			if req.Body != nil {
				_ = req.Body.Close()
			}

			readerCloser, err := req.GetBody()
			if err != nil {
				return nil, err
			}

			req.Body = readerCloser
		} else {
			req.Body = io.NopCloser(bytes.NewBuffer(body))
		}

		response, respErr = p.transport.RoundTrip(req)

		retry, checkErr = retryTransOption.checkRetryFunc(req.Context(), response, respErr)
		if respErr != nil || retry == true || checkErr != nil {
			if retryTransOption.retryErrorFunc != nil {
				response, respErr = retryTransOption.retryErrorFunc(response, respErr, i)

				retry, checkErr = retryTransOption.checkRetryFunc(req.Context(), response, respErr)
			}
		}

		if retry == false {
			break
		}

		if attemptNum >= retryTransOption.retryMaxCount {
			break
		}

		if response != nil && response.Body != nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}

		waitTime := retryTransOption.backoffFunc(retryTransOption.retryMinWaitTime, retryTransOption.retryMaxWaitTime, i, response)

		timer := time.NewTimer(waitTime)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}

	return response, errors.Join(respErr, checkErr)
}

// wrapRetryTransport returns a retrying decorator around transport, or [http.DefaultTransport] when transport is nil.
func wrapRetryTransport(transport http.RoundTripper, retryTransOption *RetryTransOption) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}

	if retryTransOption == nil {
		retryTransOption = DefaultRetryTransOption
	}

	retryTransport := &retryTransport{
		transport:        transport,
		retryTransOption: retryTransOption,
	}

	return retryTransport
}
