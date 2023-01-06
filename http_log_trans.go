/**
 * @Author: lidonglin
 * @Description:
 * @File:  http_transport
 * @Version: 1.0.0
 * @Date: 2022/07/13 15:58
 */

package thttp

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/choveylee/tlog"
)

type LogTransOption struct {
	enableSlowLog  bool
	ignoreNotFound bool
	slowLatency    time.Duration

	enableAccessLog bool
	includeHeaders  bool
}

func NewLogTransOption() *LogTransOption {
	return &LogTransOption{
		enableSlowLog:  true,
		ignoreNotFound: false,
		slowLatency:    500 * time.Millisecond,

		enableAccessLog: false,
		includeHeaders:  false,
	}
}

func (p *LogTransOption) WithSlowLog(enableSlowLog bool, slowLatency time.Duration) *LogTransOption {
	p.enableSlowLog = enableSlowLog
	p.slowLatency = slowLatency

	return p
}

func (p *LogTransOption) IgnoreNotFound(ignoreNotFound bool) *LogTransOption {
	p.ignoreNotFound = ignoreNotFound

	return p
}

func (p *LogTransOption) WithAccessLog(enableAccessLog bool) *LogTransOption {
	p.enableAccessLog = enableAccessLog

	return p
}

func (p *LogTransOption) IncludeHeaders(includeHeaders bool) *LogTransOption {
	p.includeHeaders = includeHeaders

	return p
}

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
	if p.logTransOption.enableSlowLog == true {
		if err == nil && (resp.StatusCode == http.StatusOK || (resp.StatusCode == http.StatusNotFound && p.logTransOption.ignoreNotFound == false)) {
			if latency > p.logTransOption.slowLatency {
				tlog.I(req.Context()).Err(err).Detailf("req.method: %s", req.Method).
					Detailf("req.host: %s", req.Host).Detailf("req.url: %s", req.URL.String()).
					Detailf("latency: %d", latency).Msg("slow log")
			}
		}
	}

	// add access log
	if p.logTransOption.enableAccessLog == true {
		event := tlog.I(req.Context()).Err(err).Detailf("req.method: %s", req.Method).
			Detailf("req.host: %s", req.Host).Detailf("req.url: %s", req.URL.String()).
			Detailf("latency: %d", latency)

		if p.logTransOption.includeHeaders == true {
			for key, vals := range req.Header {
				event = event.Detailf("req.header.%s: %s", key, strings.Join(vals, ";"))
			}

			for key, vals := range resp.Header {
				event = event.Detailf("resp.header.%s: %s", key, strings.Join(vals, ";"))
			}
		}

		event.Msg("access log")
	}

	return resp, err
}

func wrapLogTransport(transport http.RoundTripper, logTransOption *LogTransOption) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}

	logTransport := &logTransport{
		transport:      transport,
		logTransOption: logTransOption,
	}

	return logTransport
}
