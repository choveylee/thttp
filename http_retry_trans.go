/**
 * @Author: lidonglin
 * @Description:
 * @File:  http_retry.go
 * @Version: 1.0.0
 * @Date: 2022/05/28 10:46
 */

package thttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

const (
	DefaultRetryMaxCount    = 3
	DefaultRetryMinWaitTime = time.Duration(100) * time.Millisecond
	DefaultRetryMaxWaitTime = time.Duration(2000) * time.Millisecond
)

type CheckRetryFunc func(ctx context.Context, resp *http.Response, err error) (bool, error)

type BackoffFunc func(minWaitTime, maxWaitTime time.Duration, attemptNum int, resp *http.Response) time.Duration

type RetryErrorFunc func(resp *http.Response, err error, retryNum int) (*http.Response, error)

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

func NewRetryTransOption() *RetryTransOption {
	return &RetryTransOption{
		retryMaxCount:    DefaultRetryMaxCount,
		retryMinWaitTime: DefaultRetryMinWaitTime,
		retryMaxWaitTime: DefaultRetryMaxWaitTime,

		checkRetryFunc: DefaultRetryPolicy,
		backoffFunc:    DefaultBackoff,
	}
}

func (p *RetryTransOption) WithMaxCount(maxCount int) *RetryTransOption {
	p.retryMaxCount = maxCount

	return p
}

func (p *RetryTransOption) WithWaitTime(minWaitTime, maxWaitTime time.Duration) *RetryTransOption {
	if maxWaitTime >= minWaitTime {
		p.retryMinWaitTime = minWaitTime
		p.retryMaxWaitTime = maxWaitTime
	}

	return p
}

func (p *RetryTransOption) WithCheckRetry(checkRetryFunc CheckRetryFunc) *RetryTransOption {
	p.checkRetryFunc = checkRetryFunc

	return p
}

func (p *RetryTransOption) WithBackoff(backoffFunc BackoffFunc) *RetryTransOption {
	p.backoffFunc = backoffFunc

	return p
}

func (p *RetryTransOption) WithRetryError(retryErrorFunc RetryErrorFunc) *RetryTransOption {
	p.retryErrorFunc = retryErrorFunc

	return p
}

var DefaultRetryTransOption = &RetryTransOption{
	retryMaxCount:    DefaultRetryMaxCount,
	retryMinWaitTime: DefaultRetryMinWaitTime,
	retryMaxWaitTime: DefaultRetryMaxWaitTime,

	checkRetryFunc: DefaultRetryPolicy,
	backoffFunc:    DefaultBackoff,
}

type retryTransport struct {
	transport http.RoundTripper

	retryTransOption *RetryTransOption
}

func (p *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var response *http.Response
	var respErr, checkErr error
	var retry bool

	attemptNum := 0

	retryTransOption := p.retryTransOption

	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
	}

	for i := 0; ; i++ {
		attemptNum++

		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

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

		waitTime := retryTransOption.backoffFunc(retryTransOption.retryMinWaitTime, retryTransOption.retryMaxWaitTime, i, response)

		timer := time.NewTimer(waitTime)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}

	return response, respErr
}

func wrapRetryTransport(transport http.RoundTripper, retryTransOption *RetryTransOption) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}

	retryTransport := &retryTransport{
		transport:        transport,
		retryTransOption: retryTransOption,
	}

	return retryTransport
}
