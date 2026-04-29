package thttp

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
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
		var val *url.Error
		if errors.As(err, &val) {
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

			var unknownAuthorityError x509.UnknownAuthorityError
			if errors.As(val.Err, &unknownAuthorityError) {
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
			if srcRetryAfter, ok := resp.Header["Retry-After"]; ok {
				retryAfter, ok := parseRetryAfter(srcRetryAfter[0], time.Now())
				if ok {
					return retryAfter
				}
			}
		}
	}

	duration := math.Pow(2, float64(attemptNum)) * float64(minWaitTime)

	sleepTime := time.Duration(duration)

	if float64(sleepTime) != duration || sleepTime > maxWaitTime {
		sleepTime = maxWaitTime
	}

	return sleepTime
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	if sleep, err := strconv.ParseInt(value, 10, 64); err == nil {
		if sleep < 0 {
			return 0, true
		}

		return time.Second * time.Duration(sleep), true
	}

	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}

	retryAfter := retryAt.Sub(now)
	if retryAfter < 0 {
		return 0, true
	}

	return retryAfter, true
}

// LinearJitterBackoff chooses a pseudo-random base delay between minWaitTime and maxWaitTime,
// then multiplies it by the 1-based attempt number to produce a linearly increasing jittered
// backoff. When maxWaitTime is not greater than minWaitTime, it falls back to
// minWaitTime * attemptNum. The resp parameter is unused.
func LinearJitterBackoff(minWaitTime, maxWaitTime time.Duration, attemptNum int, resp *http.Response) time.Duration {
	// attemptNum always starts at zero but we want to start at 1 for multiplication
	attemptNum++

	if maxWaitTime <= minWaitTime {
		// When there is no valid jitter range, use minWaitTime scaled by the 1-based attempt count.
		return minWaitTime * time.Duration(attemptNum)
	}

	// Seed randVal; doing this every time is fine
	randVal := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))

	// Pick a random base delay in [minWaitTime, maxWaitTime), then scale it by the
	// 1-based attempt count to get a linearly increasing jittered delay.
	jitter := randVal.Float64() * float64(maxWaitTime-minWaitTime)
	jitterMin := int64(jitter) + int64(minWaitTime)

	return time.Duration(jitterMin * int64(attemptNum))
}
