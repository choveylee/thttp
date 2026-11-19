package thttp

import (
	"net/http"
	"strconv"
	"time"

	"github.com/choveylee/tlog"
)

// LogTransOption configures the logging [http.RoundTripper] wrapper applied when [OptTransLog] is set.
type LogTransOption struct {
	enableSlowLog  bool
	ignoreNotFound bool
	slowLatency    time.Duration

	enableAccessLog bool
	includeHeaders  bool
}

// NewLogTransOption returns defaults: slow-request logging enabled, 500 ms threshold, access logs disabled.
func NewLogTransOption() *LogTransOption {
	return &LogTransOption{
		enableSlowLog:  true,
		ignoreNotFound: false,
		slowLatency:    500 * time.Millisecond,

		enableAccessLog: false,
		includeHeaders:  false,
	}
}

// WithSlowLog enables or disables logging when round-trip latency exceeds slowLatency. Slow responses of any
// HTTP status are eligible; [LogTransOption.IgnoreNotFound] can be used to suppress slow HTTP 404 logs.
func (options *LogTransOption) WithSlowLog(enableSlowLog bool, slowLatency time.Duration) *LogTransOption {
	options.enableSlowLog = enableSlowLog
	options.slowLatency = slowLatency

	return options
}

// IgnoreNotFound controls whether slow HTTP 404 responses are excluded from slow-log eligibility.
func (options *LogTransOption) IgnoreNotFound(ignoreNotFound bool) *LogTransOption {
	options.ignoreNotFound = ignoreNotFound

	return options
}

// WithAccessLog enables one log entry per attempt with method, host and latency.
func (options *LogTransOption) WithAccessLog(enableAccessLog bool) *LogTransOption {
	options.enableAccessLog = enableAccessLog

	return options
}

// IncludeHeaders adds request and response header names to access logs. Values are redacted.
func (options *LogTransOption) IncludeHeaders(includeHeaders bool) *LogTransOption {
	options.includeHeaders = includeHeaders

	return options
}

// logTransport records latency histograms and optional structured logs around a delegate [http.RoundTripper].
type logTransport struct {
	transport http.RoundTripper

	logTransOption *LogTransOption
}

var defaultLogTransOption = &LogTransOption{
	enableSlowLog:  true,
	ignoreNotFound: false,
	slowLatency:    500 * time.Millisecond,

	enableAccessLog: false,
	includeHeaders:  false,
}

// RoundTrip measures the time until the response headers or a transport error.
func (transport *logTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	startedAt := time.Now()
	response, err := transport.transport.RoundTrip(request)
	latency := time.Since(startedAt)

	status := "-1"
	if err == nil && response != nil {
		status = strconv.Itoa(response.StatusCode)
	}

	_ = httpClientRequestHistogram.Observe(float64(latency)/float64(time.Millisecond), request.Method, status, request.Host)

	option := transport.logTransOption
	ignoreSlowLog := option.ignoreNotFound && err == nil && response != nil && response.StatusCode == http.StatusNotFound
	if option.enableSlowLog && latency > option.slowLatency && !ignoreSlowLog {
		event := withRequestDetails(tlog.I(request.Context()), request, err).
			Detailf("latency_ms: %d", latency.Milliseconds()).Detailf("resp.status code: %s", status)

		event.Msg("thttp slow request observed")
	}

	if option.enableAccessLog {
		event := withRequestDetails(tlog.I(request.Context()), request, err).
			Detailf("latency_ms: %d", latency.Milliseconds()).Detailf("resp.status code: %s", status)
		if option.includeHeaders {
			addHeaderNames(event, "req", request.Header)
			if response != nil {
				addHeaderNames(event, "resp", response.Header)
			}
		}

		event.Msg("thttp request access log entry")
	}

	return response, err
}

// withRequestDetails omits URL paths, credentials and error text, which may
// contain request data or URLs from redirects and proxies.
func withRequestDetails(event *tlog.Tevent, request *http.Request, err error) *tlog.Tevent {
	event.Detailf("req.method: %s", request.Method).Detailf("req.host: %s", request.Host)
	if err != nil {
		event.Detailf("error type: %T", err)
	}

	return event
}

// addHeaderNames logs header names with a constant redaction marker.
func addHeaderNames(event *tlog.Tevent, prefix string, headerMap http.Header) {
	for name := range headerMap {
		event.Detailf("%s.header.%s: [REDACTED]", prefix, name)
	}
}

// wrapLogTransport returns a logging decorator around transport, or [http.DefaultTransport] when transport is nil.
func wrapLogTransport(transport http.RoundTripper, logTransOption *LogTransOption) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}

	if logTransOption == nil {
		logTransOption = defaultLogTransOption
	}

	logTransport := &logTransport{
		transport:      transport,
		logTransOption: logTransOption,
	}

	return logTransport
}
