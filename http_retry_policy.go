package thttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math"
	"math/rand/v2"
	"net"
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

// DefaultRetryPolicy retries transport failures, HTTP 429 and most 5xx responses.
// Cancellation and known permanent transport errors stop retries.
func DefaultRetryPolicy(ctx context.Context, response *http.Response, err error) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if err != nil {
		return isRetryableTransportError(err), nil
	}

	if response == nil {
		return false, nil
	}

	status := response.StatusCode

	return status == http.StatusTooManyRequests || (status >= 500 && status <= 599 && status != http.StatusNotImplemented), nil
}

// isRetryableTransportError excludes failures that a second attempt cannot fix.
func isRetryableTransportError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, http.ErrSchemeMismatch) {
		return false
	}

	var dnsError *net.DNSError
	if errors.As(err, &dnsError) && dnsError.IsNotFound {
		return false
	}

	var verificationError *tls.CertificateVerificationError
	var authorityError x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	var certificateError x509.CertificateInvalidError
	if errors.As(err, &verificationError) || errors.As(err, &authorityError) ||
		errors.As(err, &hostnameError) || errors.As(err, &certificateError) {
		return false
	}

	// Match net/http error text against the cause, excluding the URL itself.
	var urlError *url.Error
	if errors.As(err, &urlError) && urlError.Err != nil {
		err = urlError.Err
	}

	message := err.Error()

	return !RedirectErrorReg.MatchString(message) && !SchemeErrorReg.MatchString(message) && !NotTrustedErrorReg.MatchString(message) &&
		!strings.HasPrefix(message, "net/http: invalid ")
}

// DefaultBackoff performs exponential backoff capped at maxWaitTime. A valid
// Retry-After on HTTP 429 or 503 takes precedence over the local schedule.
func DefaultBackoff(minWaitTime, maxWaitTime time.Duration, attemptNum int, response *http.Response) time.Duration {
	if response != nil && (response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusServiceUnavailable) {
		retryAfter, ok := parseRetryAfter(response.Header.Get("Retry-After"), time.Now())
		if ok {
			return retryAfter
		}
	}

	if minWaitTime <= 0 || maxWaitTime <= 0 {
		return 0
	}

	shift := max(attemptNum, 0)
	if shift >= 63 || minWaitTime > maxWaitTime>>shift {
		return maxWaitTime
	}

	return minWaitTime << shift
}

// parseRetryAfter accepts HTTP dates and delay-seconds without wrapping long delays.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	seconds, err := strconv.ParseUint(value, 10, 64)
	digitsOnly := strings.Trim(value, "0123456789") == ""
	if err == nil || (digitsOnly && errors.Is(err, strconv.ErrRange)) {
		if seconds > uint64(math.MaxInt64/int64(time.Second)) {
			return time.Duration(math.MaxInt64), true
		}

		return time.Duration(seconds) * time.Second, true
	}

	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}

	return max(retryAt.Sub(now), 0), true
}

// LinearJitterBackoff multiplies a random delay in the given range by the
// 1-based attempt number. Degenerate ranges use the nonnegative minimum.
func LinearJitterBackoff(minWaitTime, maxWaitTime time.Duration, attemptNum int, _ *http.Response) time.Duration {
	delay := max(minWaitTime, 0)
	if maxWaitTime > delay {
		delay += time.Duration(rand.Int64N(int64(maxWaitTime - delay)))
	}

	attempts := uint64(max(attemptNum, 0)) + 1
	if uint64(delay) > math.MaxInt64/attempts {
		return time.Duration(math.MaxInt64)
	}

	return time.Duration(uint64(delay) * attempts)
}
