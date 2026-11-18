package thttp

import (
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// RequestOption carries per-request options, headers, and cookies merged with [HttpClient] defaults.
// Per-request option keys must be set only through typed helpers (e.g. [RequestOption.WithLogTransOption]);
// there is no generic option setter so [*LogTransOption] / [*RetryTransOption] are always shallow-copied when stored.
type RequestOption struct {
	optionMap map[int]any

	// Headers contains explicit header overrides. Use the setters when sharing options.
	Headers map[string]string

	// Cookies contains scoped cookies for the outgoing request and its redirects.
	Cookies []*http.Cookie

	sync.Mutex
}

type requestOptionSnapshot struct {
	optionMap map[int]any
	headerMap map[string]string
	cookies   []*http.Cookie
}

// NewRequestOption returns an empty [RequestOption] ready for configuration.
func NewRequestOption() *RequestOption {
	requestOption := &RequestOption{
		optionMap: make(map[int]any),

		Headers: make(map[string]string),

		Cookies: make([]*http.Cookie, 0),
	}

	return requestOption
}

// snapshot copies the options used by one request under the options lock.
func (p *RequestOption) snapshot() requestOptionSnapshot {
	p.Lock()
	defer p.Unlock()

	optionMap := maps.Clone(p.optionMap)
	headerMap := make(map[string]string, len(p.Headers))
	for headerName, value := range p.Headers {
		headerMap[strings.ToLower(headerName)] = value
	}

	snapshot := requestOptionSnapshot{
		optionMap: optionMap,
		headerMap: headerMap,
		cookies:   slices.Clone(p.Cookies),
	}

	return snapshot
}

// setOption stores a typed per-request option under the options lock.
func (p *RequestOption) setOption(key int, value any) *RequestOption {
	p.Lock()
	defer p.Unlock()

	if p.optionMap == nil {
		p.optionMap = make(map[int]any)
	}

	p.optionMap[key] = cloneOptionValue(key, value)

	return p
}

// WithTimeout sets [OptTimeout] for this request only.
func (p *RequestOption) WithTimeout(timeout time.Duration) *RequestOption {
	return p.setOption(OptTimeout, timeout)
}

// WithRetryTransOption attaches retry behavior for this request ([OptTransRetry]).
func (p *RequestOption) WithRetryTransOption(option *RetryTransOption) *RequestOption {
	return p.setOption(OptTransRetry, option)
}

// WithLogTransOption attaches logging behavior for this request ([OptTransLog]).
func (p *RequestOption) WithLogTransOption(option *LogTransOption) *RequestOption {
	return p.setOption(OptTransLog, option)
}

// WithCookieJar sets the cookie jar for this request. Nil disables stored cookies.
func (p *RequestOption) WithCookieJar(jar http.CookieJar) *RequestOption {
	return p.setOption(OptCookieJar, jar)
}

// WithRedirectPolicy sets the redirect policy for this request ([OptRedirectPolicy]).
func (p *RequestOption) WithRedirectPolicy(option RedirectPolicyFunc) *RequestOption {
	return p.setOption(OptRedirectPolicy, option)
}

// WithRequestHookFunc registers a pre-request hook for this request ([OptExtraRequestHookFunc]).
func (p *RequestOption) WithRequestHookFunc(option RequestHookFunc) *RequestOption {
	return p.setOption(OptExtraRequestHookFunc, option)
}

// WithResponseHookFunc registers a post-request hook for this request ([OptExtraResponseHookFunc]).
func (p *RequestOption) WithResponseHookFunc(option ResponseHookFunc) *RequestOption {
	return p.setOption(OptExtraResponseHookFunc, option)
}

// WithHeader sets a header for this request.
func (p *RequestOption) WithHeader(key string, value string) *RequestOption {
	return p.WithHeaders(map[string]string{key: value})
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

// WithHeaders merges headers for this request under one lock.
func (p *RequestOption) WithHeaders(headerMap map[string]string) *RequestOption {
	p.Lock()
	defer p.Unlock()

	if p.Headers == nil {
		p.Headers = make(map[string]string, len(headerMap))
	}

	for headerName, value := range headerMap {
		p.Headers[strings.ToLower(headerName)] = value
	}

	return p
}

// WithCookie appends a scoped cookie for the request.
func (p *RequestOption) WithCookie(cookie *http.Cookie) *RequestOption {
	return p.WithCookies(cookie)
}

// WithCookies appends scoped cookies for the request.
func (p *RequestOption) WithCookies(cookies ...*http.Cookie) *RequestOption {
	p.Lock()
	defer p.Unlock()

	p.Cookies = append(p.Cookies, cookies...)

	return p
}
