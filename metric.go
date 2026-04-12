package thttp

import (
	"github.com/choveylee/tmetric"
)

// httpClientRequestHistogram records per-request latency in milliseconds by method, status, and host.
var (
	httpClientRequestHistogram, _ = tmetric.NewHistogramVec(
		"http_client_request_latency",
		"time between first byte of request headers sent to last byte of response received, or terminal error",
		[]string{
			"http_client_method",
			"http_client_status",
			"http_client_host",
		},
	)
)
