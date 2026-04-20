package thttp

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// RequestOption carries per-request options, headers, and cookies merged with [HttpClient] defaults.
type RequestOption struct {
	Options map[int]interface{}

	Headers map[string]string

	Cookies []*http.Cookie

	sync.Mutex
}

// NewRequestOption returns an empty [RequestOption] ready for configuration.
func NewRequestOption() *RequestOption {
	requestOption := &RequestOption{
		Options: make(map[int]interface{}),

		Headers: make(map[string]string),

		Cookies: make([]*http.Cookie, 0),
	}

	return requestOption
}

// WithOption sets a per-request option. Keys listed in [OptTransports] are ignored (transport fields are client-wide).
// [*LogTransOption] and [*RetryTransOption] are shallow-copied when stored so callers can reuse the same template safely.
func (p *RequestOption) WithOption(key int, val interface{}) *RequestOption {
	p.Lock()
	defer p.Unlock()

	_, ok := OptTransports[key]
	if ok == true {
		return p
	}

	if p.Options == nil {
		p.Options = make(map[int]interface{})
	}

	p.Options[key] = cloneOptionValue(key, val)

	return p
}

// WithTimeout sets [OptTimeout] for this request only.
func (p *RequestOption) WithTimeout(timeout time.Duration) *RequestOption {
	return p.WithOption(OptTimeout, timeout)
}

// WithRetryTransOption attaches retry behavior for this request ([OptTransRetry]).
func (p *RequestOption) WithRetryTransOption(option *RetryTransOption) *RequestOption {
	return p.WithOption(OptTransRetry, option)
}

// WithLogTransOption attaches logging behavior for this request ([OptTransLog]).
func (p *RequestOption) WithLogTransOption(option *LogTransOption) *RequestOption {
	return p.WithOption(OptTransLog, option)
}

// WithCookieJar sets the cookie jar for this request ([OptCookieJar]).
func (p *RequestOption) WithCookieJar(jar http.CookieJar) *RequestOption {
	return p.WithOption(OptCookieJar, jar)
}

// WithRedirectPolicy sets the redirect policy for this request ([OptRedirectPolicy]).
func (p *RequestOption) WithRedirectPolicy(option RedirectPolicyFunc) *RequestOption {
	return p.WithOption(OptRedirectPolicy, option)
}

// WithRequestHookFunc registers a pre-request hook for this request ([OptExtraRequestHookFunc]).
func (p *RequestOption) WithRequestHookFunc(option RequestHookFunc) *RequestOption {
	return p.WithOption(OptExtraRequestHookFunc, option)
}

// WithResponseHookFunc registers a post-request hook for this request ([OptExtraResponseHookFunc]).
func (p *RequestOption) WithResponseHookFunc(option ResponseHookFunc) *RequestOption {
	return p.WithOption(OptExtraResponseHookFunc, option)
}

// WithOptions merges multiple per-request options.
func (p *RequestOption) WithOptions(options map[int]interface{}) *RequestOption {
	for key, val := range options {
		p.WithOption(key, val)
	}

	return p
}

// WithHeader sets a header for this request (key is stored in lowercase).
func (p *RequestOption) WithHeader(key string, val string) *RequestOption {
	p.Lock()
	defer p.Unlock()

	if p.Headers == nil {
		p.Headers = make(map[string]string)
	}

	p.Headers[strings.ToLower(key)] = val

	return p
}

// WithReferer sets the Referer header for this request.
func (p *RequestOption) WithReferer(val string) *RequestOption {
	return p.WithHeader("referer", val)
}

// WithUserAgent sets the User-Agent header for this request.
func (p *RequestOption) WithUserAgent(val string) *RequestOption {
	return p.WithHeader("user-agent", val)
}

// WithContentType sets the Content-Type header for this request.
func (p *RequestOption) WithContentType(val string) *RequestOption {
	return p.WithHeader("content-type", val)
}

// WithHeaders merges multiple headers for this request.
func (p *RequestOption) WithHeaders(headers map[string]string) *RequestOption {
	for key, val := range headers {
		p.WithHeader(key, val)
	}

	return p
}

// WithCookie appends a single cookie to the request.
func (p *RequestOption) WithCookie(cookie *http.Cookie) *RequestOption {
	p.Lock()
	defer p.Unlock()

	p.Cookies = append(p.Cookies, cookie)

	return p
}

// WithCookies appends multiple cookies to the request.
func (p *RequestOption) WithCookies(cookies ...*http.Cookie) *RequestOption {
	p.Lock()
	defer p.Unlock()

	p.Cookies = append(p.Cookies, cookies...)

	return p
}
