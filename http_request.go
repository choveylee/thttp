/**
 * @Author: lidonglin
 * @Description:
 * @File:  http_request.go
 * @Version: 1.0.0
 * @Date: 2022/06/05 11:49
 */

package thttp

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type RequestOption struct {
	Options map[int]interface{}

	Headers map[string]string

	Cookies []*http.Cookie

	sync.Mutex
}

func NewRequestOption() *RequestOption {
	requestOption := &RequestOption{
		Options: make(map[int]interface{}),

		Headers: make(map[string]string),

		Cookies: make([]*http.Cookie, 0),
	}

	return requestOption
}

func (p *RequestOption) WithOption(key int, val interface{}) *RequestOption {
	p.Lock()
	defer p.Unlock()

	_, ok := OptTransports[key]
	if ok == true {
		return p
	}

	p.Options[key] = val

	return p
}

// WithTimeout timeout option
func (p *RequestOption) WithTimeout(timeout time.Duration) *RequestOption {
	return p.WithOption(OptTimeout, timeout)
}

// WithRetryTransOption retry trans option
func (p *RequestOption) WithRetryTransOption(option *RetryTransOption) *RequestOption {
	return p.WithOption(OptTransRetry, option)
}

// WithLogTransOption log trans option
func (p *RequestOption) WithLogTransOption(option *LogTransOption) *RequestOption {
	return p.WithOption(OptTransLog, option)
}

// WithCookieJar cookie jar
func (p *RequestOption) WithCookieJar(jar http.CookieJar) *RequestOption {
	return p.WithOption(OptCookieJar, jar)
}

// WithRedirectPolicy redirect policy
func (p *RequestOption) WithRedirectPolicy(option RedirectPolicyFunc) *RequestOption {
	return p.WithOption(OptRedirectPolicy, option)
}

// WithRequestHookFunc request hook func
func (p *RequestOption) WithRequestHookFunc(option RequestHookFunc) *RequestOption {
	return p.WithOption(OptExtraRequestHookFunc, option)
}

// WithResponseHookFunc response hook func
func (p *RequestOption) WithResponseHookFunc(option ResponseHookFunc) *RequestOption {
	return p.WithOption(OptExtraResponseHookFunc, option)
}

func (p *RequestOption) WithOptions(options map[int]interface{}) *RequestOption {
	for key, val := range options {
		p.WithOption(key, val)
	}

	return p
}

func (p *RequestOption) WithHeader(key string, val string) *RequestOption {
	p.Lock()
	defer p.Unlock()

	p.Headers[strings.ToLower(key)] = val

	return p
}

func (p *RequestOption) WithReferer(val string) *RequestOption {
	return p.WithHeader("referer", val)
}

func (p *RequestOption) WithUserAgent(val string) *RequestOption {
	return p.WithHeader("user-agent", val)
}

func (p *RequestOption) WithContentType(val string) *RequestOption {
	return p.WithHeader("content-type", val)
}

func (p *RequestOption) WithHeaders(headers map[string]string) *RequestOption {
	for key, val := range headers {
		p.WithHeader(key, val)
	}

	return p
}

func (p *RequestOption) WithCookie(cookie *http.Cookie) *RequestOption {
	p.Lock()
	defer p.Unlock()

	p.Cookies = append(p.Cookies, cookie)

	return p
}

func (p *RequestOption) WithCookies(cookies ...*http.Cookie) *RequestOption {
	p.Lock()
	defer p.Unlock()

	p.Cookies = append(p.Cookies, cookies...)

	return p
}
