package thttp

import (
	"fmt"
	"net/http"
	"strings"
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
func (p *LogTransOption) WithSlowLog(enableSlowLog bool, slowLatency time.Duration) *LogTransOption {
	p.enableSlowLog = enableSlowLog
	p.slowLatency = slowLatency

	return p
}

// IgnoreNotFound controls whether slow HTTP 404 responses are excluded from slow-log eligibility.
func (p *LogTransOption) IgnoreNotFound(ignoreNotFound bool) *LogTransOption {
	p.ignoreNotFound = ignoreNotFound

	return p
}

// WithAccessLog enables one line per request with method, host, URL, and latency.
func (p *LogTransOption) WithAccessLog(enableAccessLog bool) *LogTransOption {
	p.enableAccessLog = enableAccessLog

	return p
}

// IncludeHeaders adds request and response headers to access logs when access logging is enabled.
func (p *LogTransOption) IncludeHeaders(includeHeaders bool) *LogTransOption {
	p.includeHeaders = includeHeaders

	return p
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

func shouldEmitSlowLog(resp *http.Response, err error, ignoreNotFound bool) bool {
	if err != nil || resp == nil {
		return true
	}

	if ignoreNotFound && resp.StatusCode == http.StatusNotFound {
		return false
	}

	return true
}

// RoundTrip implements [http.RoundTripper].
func (p *logTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	startedAt := time.Now()
	resp, err := p.transport.RoundTrip(req)
	latency := time.Since(startedAt)

	if err != nil {
		httpClientRequestHistogram.Observe(float64(latency)/float64(time.Millisecond), req.Method, fmt.Sprint(-1), req.Host)
	} else {
		httpClientRequestHistogram.Observe(float64(latency)/float64(time.Millisecond), req.Method, fmt.Sprint(resp.StatusCode), req.Host)
	}

	// add slow log
	if p.logTransOption.enableSlowLog == true &&
		latency > p.logTransOption.slowLatency &&
		shouldEmitSlowLog(resp, err, p.logTransOption.ignoreNotFound) {
		event := tlog.I(req.Context()).Err(err).Detailf("req.method: %s", req.Method).
			Detailf("req.host: %s", req.Host).Detailf("req.url: %s", req.URL.String()).
			Detailf("latency_ms: %d", latency.Milliseconds())

		if resp != nil {
			event = event.Detailf("resp.status code: %d", resp.StatusCode)
		}

		event.Msg("thttp slow request detected")
	}

	// add access log
	if p.logTransOption.enableAccessLog == true {
		event := tlog.I(req.Context()).Err(err).Detailf("req.method: %s", req.Method).
			Detailf("req.host: %s", req.Host).Detailf("req.url: %s", req.URL.String()).
			Detailf("latency_ms: %d", latency.Milliseconds())

		if p.logTransOption.includeHeaders == true {
			for key, vals := range req.Header {
				event = event.Detailf("req.header.%s: %s", key, strings.Join(vals, ";"))
			}

			if resp != nil {
				for key, vals := range resp.Header {
					event = event.Detailf("resp.header.%s: %s", key, strings.Join(vals, ";"))
				}
			}
		}

		event.Msg("thttp access log entry")
	}

	return resp, err
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
