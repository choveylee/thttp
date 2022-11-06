/**
 * @Author: lidonglin
 * @Description:
 * @File:  metric.go
 * @Version: 1.0.0
 * @Date: 2022/07/14 10:58
 */

package thttp

import (
	"github.com/choveylee/tmetric"
)

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
