package thttp

import (
	"github.com/choveylee/tmetric"
)

// httpClientRequestHistogram records transport attempt latency in milliseconds.
var httpClientRequestHistogram, _ = tmetric.NewHistogramVec(
	"http_client_request_latency",
	"HTTP transport attempt duration in milliseconds until response headers or a transport error",
	[]string{"http_client_method", "http_client_status", "http_client_host"},
)
