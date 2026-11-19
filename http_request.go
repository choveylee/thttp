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
func (options *RequestOption) snapshot() requestOptionSnapshot {
	options.Lock()
	defer options.Unlock()

	optionMap := maps.Clone(options.optionMap)
	headerMap := make(map[string]string, len(options.Headers))
	for headerName, value := range options.Headers {
		headerMap[strings.ToLower(headerName)] = value
	}

	snapshot := requestOptionSnapshot{
		optionMap: optionMap,
		headerMap: headerMap,
		cookies:   slices.Clone(options.Cookies),
	}

	return snapshot
}

// setOption stores a typed per-request option under the options lock.
func (options *RequestOption) setOption(key int, value any) *RequestOption {
	options.Lock()
	defer options.Unlock()

	if options.optionMap == nil {
		options.optionMap = make(map[int]any)
	}

	options.optionMap[key] = cloneOptionValue(key, value)

	return options
}

// WithTimeout sets [OptTimeout] for this request only.
func (options *RequestOption) WithTimeout(timeout time.Duration) *RequestOption {
	return options.setOption(OptTimeout, timeout)
}

// WithRetryTransOption attaches retry behavior for this request ([OptTransRetry]).
func (options *RequestOption) WithRetryTransOption(option *RetryTransOption) *RequestOption {
	return options.setOption(OptTransRetry, option)
}

// WithLogTransOption attaches logging behavior for this request ([OptTransLog]).
func (options *RequestOption) WithLogTransOption(option *LogTransOption) *RequestOption {
	return options.setOption(OptTransLog, option)
}

// WithCookieJar sets the cookie jar for this request. Nil disables stored cookies.
func (options *RequestOption) WithCookieJar(jar http.CookieJar) *RequestOption {
	return options.setOption(OptCookieJar, jar)
}

// WithRedirectPolicy sets the redirect policy for this request ([OptRedirectPolicy]).
func (options *RequestOption) WithRedirectPolicy(option RedirectPolicyFunc) *RequestOption {
	return options.setOption(OptRedirectPolicy, option)
}

// WithRequestHookFunc registers a pre-request hook for this request ([OptExtraRequestHookFunc]).
func (options *RequestOption) WithRequestHookFunc(option RequestHookFunc) *RequestOption {
	return options.setOption(OptExtraRequestHookFunc, option)
}

// WithResponseHookFunc registers a post-request hook for this request ([OptExtraResponseHookFunc]).
func (options *RequestOption) WithResponseHookFunc(option ResponseHookFunc) *RequestOption {
	return options.setOption(OptExtraResponseHookFunc, option)
}

// WithHeader sets a header for this request.
func (options *RequestOption) WithHeader(key string, value string) *RequestOption {
	return options.WithHeaders(map[string]string{key: value})
}

// WithReferer sets the Referer header for this request.
func (options *RequestOption) WithReferer(val string) *RequestOption {
	return options.WithHeader("referer", val)
}

// WithUserAgent sets the User-Agent header for this request.
func (options *RequestOption) WithUserAgent(val string) *RequestOption {
	return options.WithHeader("user-agent", val)
}

// WithContentType sets the Content-Type header for this request.
func (options *RequestOption) WithContentType(val string) *RequestOption {
	return options.WithHeader("content-type", val)
}

// WithHeaders merges headers for this request under one lock.
func (options *RequestOption) WithHeaders(headerMap map[string]string) *RequestOption {
	options.Lock()
	defer options.Unlock()

	if options.Headers == nil {
		options.Headers = make(map[string]string, len(headerMap))
	}

	for headerName, value := range headerMap {
		options.Headers[strings.ToLower(headerName)] = value
	}

	return options
}

// WithCookie appends a scoped cookie for the request.
func (options *RequestOption) WithCookie(cookie *http.Cookie) *RequestOption {
	return options.WithCookies(cookie)
}

// WithCookies appends scoped cookies for the request.
func (options *RequestOption) WithCookies(cookies ...*http.Cookie) *RequestOption {
	options.Lock()
	defer options.Unlock()

	options.Cookies = append(options.Cookies, cookies...)

	return options
}
