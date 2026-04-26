package thttp

import (
	"context"
	"crypto/x509"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

var (
	// RedirectErrorReg matches the message produced by net/http when the maximum number of redirects is exceeded.
	RedirectErrorReg = regexp.MustCompile(`stopped after \d+ redirects\z`)

	// SchemeErrorReg matches the message produced for an unsupported URL scheme.
	SchemeErrorReg = regexp.MustCompile(`unsupported protocol scheme`)

	// NotTrustedErrorReg matches the message produced when a TLS certificate is not trusted.
	NotTrustedErrorReg = regexp.MustCompile(`certificate is not trusted`)
)

// baseRetryPolicy implements the default retry eligibility rules shared by higher-level policies.
func baseRetryPolicy(resp *http.Response, err error) (bool, error) {
	if err != nil {
		if val, ok := err.(*url.Error); ok {
			// Don't retry if the error was due to too many redirects.
			if RedirectErrorReg.MatchString(val.Error()) {
				return false, val
			}

			// Don't retry if the error was due to an invalid protocol scheme.
			if SchemeErrorReg.MatchString(val.Error()) {
				return false, val
			}

			// Don't retry if the error was due to TLS cert verification failure.
			if NotTrustedErrorReg.MatchString(val.Error()) {
				return false, val
			}

			if _, ok := val.Err.(x509.UnknownAuthorityError); ok {
				return false, val
			}
		}

		return true, nil
	}

	// 429 Too Many Requests is recoverable. Sometimes the server puts
	// a Retry-After response header to indicate when the server is
	// available to start processing request from client.
	if resp.StatusCode == http.StatusTooManyRequests {
		return true, nil
	}

	// Check the response code. We retry on 500-range responses to allow
	// the server time to recover, as 500's are typically not permanent
	// errors and may relate to outages on the server side. This will catch
	// invalid response codes as well, like 0 and 999.
	if resp.StatusCode == 0 || (resp.StatusCode >= 500 && resp.StatusCode != http.StatusNotImplemented) {
		return true, fmt.Errorf("thttp: retryable HTTP status %s", resp.Status)
	}

	return false, nil
}

// DefaultRetryPolicy decides whether to retry after a round trip. It returns false when the context
// is canceled or deadlined; otherwise it applies the same eligibility rules as the internal base policy.
func DefaultRetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	// do not retry on context.Canceled or context.DeadlineExceeded
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	// don't propagate other errors
	retryFlag, _ := baseRetryPolicy(resp, err)

	return retryFlag, nil
}

// DefaultBackoff performs exponential backoff capped at maxWaitTime, and honors Retry-After for
// HTTP 429 and 503 responses when present.
func DefaultBackoff(minWaitTime, maxWaitTime time.Duration, attemptNum int, resp *http.Response) time.Duration {
	if resp != nil {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			if s, ok := resp.Header["Retry-After"]; ok {
				if sleep, err := strconv.ParseInt(s[0], 10, 64); err == nil {
					return time.Second * time.Duration(sleep)
				}
			}
		}
	}

	mult := math.Pow(2, float64(attemptNum)) * float64(minWaitTime)
	sleepTime := time.Duration(mult)

	if float64(sleepTime) != mult || sleepTime > maxWaitTime {
		sleepTime = maxWaitTime
	}

	return sleepTime
}

// LinearJitterBackoff applies linearly increasing delays with pseudo-random jitter between minWaitTime
// and maxWaitTime to reduce synchronized retries. The resp parameter is unused.
func LinearJitterBackoff(minWaitTime, maxWaitTime time.Duration, attemptNum int, resp *http.Response) time.Duration {
	// attemptNum always starts at zero but we want to start at 1 for multiplication
	attemptNum++

	if maxWaitTime <= minWaitTime {
		// Unclear what to do here, or they are the same, so return minWaitTime *
		// attemptNum
		return minWaitTime * time.Duration(attemptNum)
	}

	// Seed rand; doing this every time is fine
	rand := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))

	// Pick a random number that lies somewhere between the minWaitTime and maxWaitTime and
	// multiply by the attemptNum. attemptNum starts at zero so we always
	// increment here. We first get a random percentage, then apply that to the
	// difference between minWaitTime and maxWaitTime, and add to minWaitTime.
	jitter := rand.Float64() * float64(maxWaitTime-minWaitTime)
	jitterMin := int64(jitter) + int64(minWaitTime)

	return time.Duration(jitterMin * int64(attemptNum))
}
