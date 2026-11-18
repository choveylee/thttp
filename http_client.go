package thttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	neturl "net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/choveylee/tlog"
	"golang.org/x/net/publicsuffix"
)

// Client option keys for [HttpClient.WithOption] and [RequestOption] typed helpers (e.g. [RequestOption.WithTimeout]).
const (
	// OptTimeout sets the [http.Client] total request timeout ([time.Duration]).
	OptTimeout int = iota

	// OptTransProxyUrl sets an HTTP proxy URL string (host:port; "http://" is prepended if missing).
	OptTransProxyUrl
	// OptTransProxyFunc sets a per-request proxy function (see [HttpClient.WithProxyFunc]).
	OptTransProxyFunc

	// OptTransMaxIdleConns sets [http.Transport.MaxIdleConns] (int).
	OptTransMaxIdleConns
	// OptTransMaxIdleConnsPerHost sets [http.Transport.MaxIdleConnsPerHost] (int).
	OptTransMaxIdleConnsPerHost
	// OptTransMaxConnsPerHost sets [http.Transport.MaxConnsPerHost] (int).
	OptTransMaxConnsPerHost

	// OptTransUnsafeTls sets [tls.Config.InsecureSkipVerify] on the transport (bool).
	OptTransUnsafeTls
	// OptTransTlsConfig replaces the transport TLS configuration (*[tls.Config]).
	OptTransTlsConfig

	// OptTransRetry attaches the retry [RoundTripper] with a [*RetryTransOption].
	OptTransRetry
	// OptTransLog attaches the logging [RoundTripper] with a [*LogTransOption].
	OptTransLog

	// OptCookieJar enables the default cookie jar (bool) or supplies a custom [http.CookieJar].
	OptCookieJar

	// OptRedirectPolicy sets [http.Client.CheckRedirect] (func(*http.Request, []*http.Request) error).
	OptRedirectPolicy

	// OptExtraRequestHookFunc is invoked before [http.Client.Do] (see [HttpClient.Do]).
	OptExtraRequestHookFunc

	// OptExtraResponseHookFunc is invoked after [http.Client.Do] returns.
	OptExtraResponseHookFunc
)

// A proxy function overrides a static proxy and an explicit unsafe TLS setting
// overrides the corresponding field of a TLS config in the same batch.
var transportOptionOrder = [...]int{
	OptTransProxyUrl,
	OptTransProxyFunc,
	OptTransMaxIdleConns,
	OptTransMaxIdleConnsPerHost,
	OptTransMaxConnsPerHost,
	OptTransTlsConfig,
	OptTransUnsafeTls,
}

// OptTransports lists the option keys that configure a client-wide [http.Transport].
var OptTransports = func() map[int]int {
	optionMap := make(map[int]int, len(transportOptionOrder))
	for _, key := range transportOptionOrder {
		optionMap[key] = key
	}

	return optionMap
}()

// newDefaultTransport returns an [http.Transport] with dial timeouts, HTTP/2, and idle connection defaults.
func newDefaultTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		DialContext: dialer.DialContext,

		ForceAttemptHTTP2: true,

		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,

		TLSHandshakeTimeout: 10 * time.Second,

		ExpectContinueTimeout: 1 * time.Second,
	}

	return transport
}

// ProxyFunc is the same type as [http.Transport.Proxy] (alias so values round-trip through [HttpClient.WithOption]).
type ProxyFunc = func(*http.Request) (*neturl.URL, error)

// RedirectPolicyFunc is the same type as [http.Client.CheckRedirect].
type RedirectPolicyFunc = func(*http.Request, []*http.Request) error

// RequestHookFunc is called immediately before [http.Client.Do] executes the request (alias so hooks survive [HttpClient.WithOption]).
type RequestHookFunc = func(*http.Client, *http.Request)

// ResponseHookFunc is called after [http.Client.Do] returns, receiving the response and error from the round trip.
type ResponseHookFunc = func(*http.Response, error)

// prepareRequest builds an HTTP request and preserves net/http's built-in
// GetBody support for byte and string readers.
func prepareRequest(ctx context.Context, method string, rawURL string, headerMap map[string]string, body io.Reader) (*http.Request, error) {
	value := reflect.ValueOf(body)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, errors.New("thttp: request body reader must not be nil")
	}

	request, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}

	for headerName, value := range headerMap {
		request.Header.Set(headerName, value)
	}

	return request, nil
}

type clientSnapshot struct {
	optionMap map[int]any
	headerMap map[string]string

	transport *http.Transport

	cookieJar http.CookieJar

	transportErr error

	withDebug bool
}

// wrapTransport decorates the base transport with logging and optional retries.
func wrapTransport(transport http.RoundTripper, optionMap map[int]any) (http.RoundTripper, error) {
	var logOption *LogTransOption
	logValue, ok := optionMap[OptTransLog]
	if ok {
		logOption, ok = logValue.(*LogTransOption)
		if !ok {
			return nil, fmt.Errorf("thttp: invalid OptTransLog value: want *LogTransOption, got %T", logValue)
		}
	}

	wrappedTransport := wrapLogTransport(transport, logOption)
	retryValue, ok := optionMap[OptTransRetry]
	if ok {
		retryOption, ok := retryValue.(*RetryTransOption)
		if !ok {
			return nil, fmt.Errorf("thttp: invalid OptTransRetry value: want *RetryTransOption, got %T", retryValue)
		}

		wrappedTransport = wrapRetryTransport(wrappedTransport, retryOption)
	}

	return wrappedTransport, nil
}

// newCookieJar restricts parent-domain cookies to domains below public suffixes.
func newCookieJar() (http.CookieJar, error) {
	return cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
}

// prepareCookieJar resolves the per-request jar, including an explicit nil that
// disables shared cookie storage for that request.
func prepareCookieJar(optionMap map[int]any, defaultJar http.CookieJar) (http.CookieJar, error) {
	jarValue, ok := optionMap[OptCookieJar]
	if !ok {
		return defaultJar, nil
	}

	switch jar := jarValue.(type) {
	case nil:
		return nil, nil
	case bool:
		if !jar {
			return nil, nil
		}
		if defaultJar != nil {
			return defaultJar, nil
		}

		return newCookieJar()
	case http.CookieJar:
		value := reflect.ValueOf(jar)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
			if value.IsNil() {
				return nil, nil
			}
		}

		return jar, nil
	default:
		return nil, fmt.Errorf("thttp: invalid OptCookieJar value: want bool or http.CookieJar, got %T", jarValue)
	}
}

// prepareRedirect returns the configured redirect handler or net/http's default.
func prepareRedirect(optionMap map[int]any) (RedirectPolicyFunc, error) {
	redirectValue, ok := optionMap[OptRedirectPolicy]
	if !ok {
		return nil, nil
	}

	redirectPolicy, ok := redirectValue.(RedirectPolicyFunc)
	if !ok {
		return nil, fmt.Errorf("thttp: invalid OptRedirectPolicy value: want func(*http.Request, []*http.Request) error, got %T", redirectValue)
	}

	return redirectPolicy, nil
}

// HttpClient holds default transport settings, per-client options, and default headers used by [HttpClient.Do]
// and the HTTP verb helpers.
type HttpClient struct {
	// optionMap stores [OptTimeout], hook functions, logging and retry configuration, and other non-transport keys.
	optionMap map[int]any

	// headerMap holds default header keys (lowercased) merged into every request unless overridden per call.
	headerMap map[string]string

	// transport is the base [http.Transport] mutated by transport-related options.
	transport *http.Transport

	// cookieJar is the optional default jar used when constructing clients if not overridden per request.
	cookieJar http.CookieJar

	// withDebug enables logging of outbound request metadata.
	withDebug bool

	// transportErr is set when the last [HttpClient.WithOption] or [HttpClient.Defaults] for an [OptTransports] key failed to apply.
	transportErr error

	transportInit sync.Once
	sync.RWMutex
}

// newClientSnapshot copies the request-relevant client state under [HttpClient.RLock] so [HttpClient.Do] can release the lock
// before invoking hooks or performing network I/O.
func (p *HttpClient) newClientSnapshot() clientSnapshot {
	p.RLock()
	defer p.RUnlock()

	optionMap := make(map[int]any, len(p.optionMap))
	for key, val := range p.optionMap {
		optionMap[key] = val
	}

	headerMap := make(map[string]string, len(p.headerMap))
	for key, val := range p.headerMap {
		headerMap[key] = val
	}

	return clientSnapshot{
		optionMap: optionMap,
		headerMap: headerMap,

		transport: p.transport,

		cookieJar: p.cookieJar,

		transportErr: p.transportErr,

		withDebug: p.withDebug,
	}
}

// NewHttpClient returns an [HttpClient] with a fresh transport, 30-second connect and deadline timeouts,
// and a default cookie jar when [cookiejar.New] succeeds.
// A zero [HttpClient] is usable: the first [HttpClient.Do] or [HttpClient.Transport] runs [HttpClient.lazyInitTransport]
// to initialize the transport and default cookie jar.
func NewHttpClient() *HttpClient {
	httpClient := &HttpClient{
		optionMap: make(map[int]any),
		headerMap: make(map[string]string),

		transport: newDefaultTransport(),
	}

	cookieJar, err := newCookieJar()
	if err == nil {
		httpClient.cookieJar = cookieJar
	}

	return httpClient
}

// cloneOptionValue returns a shallow copy for [*LogTransOption] and [*RetryTransOption] so stored config is not
// aliased to a template the caller may mutate from another goroutine.
func cloneOptionValue(key int, val any) any {
	switch key {
	case OptTransLog:
		logTransOption, ok := val.(*LogTransOption)
		if !ok {
			return val
		}

		if logTransOption == nil {
			return val
		}

		copied := *logTransOption

		return &copied
	case OptTransRetry:
		retryTransOption, ok := val.(*RetryTransOption)
		if !ok {
			return val
		}

		if retryTransOption == nil {
			return val
		}

		copied := *retryTransOption

		return &copied
	default:
		return val
	}
}

// Defaults merges options and default headers in a single client update.
func (p *HttpClient) Defaults(optionMap map[int]any, headerMap map[string]string) *HttpClient {
	p.Lock()
	defer p.Unlock()

	p.applyOptionsLocked(optionMap)
	p.mergeHeadersLocked(headerMap)

	return p
}

// Debug enables or disables request metadata logging for this client.
func (p *HttpClient) Debug(val bool) *HttpClient {
	p.Lock()
	defer p.Unlock()

	p.withDebug = val

	return p
}

// lazyInitTransport allocates [HttpClient.transport] once on first [HttpClient.Do] or [HttpClient.Transport].
func (p *HttpClient) lazyInitTransport() {
	p.transportInit.Do(func() {
		p.Lock()
		defer p.Unlock()

		if p.transport == nil {
			p.transport = newDefaultTransport()
		}

		if p.cookieJar == nil {
			cookieJar, err := newCookieJar()
			if err == nil {
				p.cookieJar = cookieJar
			}
		}
	})
}

// Transport returns the underlying [http.Transport] used by this client.
func (p *HttpClient) Transport() http.RoundTripper {
	p.lazyInitTransport()

	p.RLock()
	defer p.RUnlock()

	return p.transport
}

// applyOptionsLocked publishes a new transport only when every transport option
// in the batch is valid. The caller holds the client lock.
func (p *HttpClient) applyOptionsLocked(optionMap map[int]any) {
	for key, value := range optionMap {
		if slices.Contains(transportOptionOrder[:], key) {
			continue
		}

		if p.optionMap == nil {
			p.optionMap = make(map[int]any)
		}

		p.optionMap[key] = cloneOptionValue(key, value)
	}

	var transport *http.Transport
	var transportErr error
	for _, key := range transportOptionOrder {
		value, ok := optionMap[key]
		if !ok {
			continue
		}

		if transport == nil {
			if p.transport == nil {
				transport = newDefaultTransport()
			} else {
				transport = p.transport.Clone()
			}
		}

		if err := configureTransport(transport, key, value); err != nil {
			transportErr = errors.Join(transportErr, err)
			tlog.E(context.Background()).Detailf("option: %d", key).Msg("thttp transport option update failed")
		}
	}

	if transport == nil {
		return
	}

	p.transportErr = transportErr
	if transportErr == nil {
		oldTransport := p.transport
		p.transport = transport
		if oldTransport != nil {
			oldTransport.CloseIdleConnections()
		}
	}
}

// configureTransport applies one option to a privately owned transport.
func configureTransport(transport *http.Transport, key int, value any) error {
	switch key {
	case OptTransMaxIdleConns:
		count, ok := value.(int)
		if !ok {
			return fmt.Errorf("thttp: invalid OptTransMaxIdleConns value: want int, got %T", value)
		}

		transport.MaxIdleConns = count
	case OptTransMaxIdleConnsPerHost:
		count, ok := value.(int)
		if !ok {
			return fmt.Errorf("thttp: invalid OptTransMaxIdleConnsPerHost value: want int, got %T", value)
		}

		transport.MaxIdleConnsPerHost = count
	case OptTransMaxConnsPerHost:
		count, ok := value.(int)
		if !ok {
			return fmt.Errorf("thttp: invalid OptTransMaxConnsPerHost value: want int, got %T", value)
		}

		transport.MaxConnsPerHost = count
	case OptTransProxyFunc:
		proxyFunc, ok := value.(ProxyFunc)
		if !ok {
			return fmt.Errorf("thttp: invalid OptTransProxyFunc value: want ProxyFunc (func(*http.Request) (*url.URL, error)), got %T", value)
		}

		transport.Proxy = proxyFunc
	case OptTransProxyUrl:
		rawProxy, ok := value.(string)
		if !ok {
			return fmt.Errorf("thttp: invalid OptTransProxyUrl value: want string, got %T", value)
		}

		proxy, err := parseProxyURL(rawProxy)
		if err != nil {
			return err
		}

		transport.Proxy = nil
		if proxy != nil {
			transport.Proxy = http.ProxyURL(proxy)
		}
	case OptTransTlsConfig:
		config, ok := value.(*tls.Config)
		if !ok {
			return fmt.Errorf("thttp: invalid OptTransTlsConfig value: want *tls.Config, got %T", value)
		}

		transport.TLSClientConfig = config.Clone()
	case OptTransUnsafeTls:
		unsafeTLS, ok := value.(bool)
		if !ok {
			return fmt.Errorf("thttp: invalid OptTransUnsafeTls value: want bool, got %T", value)
		}

		if transport.TLSClientConfig == nil {
			config := &tls.Config{}

			transport.TLSClientConfig = config
		}

		transport.TLSClientConfig.InsecureSkipVerify = unsafeTLS
	}

	return nil
}

// parseProxyURL accepts a proxy URL or a host and port. An empty value clears it.
func parseProxyURL(rawProxy string) (*neturl.URL, error) {
	if rawProxy == "" {
		return nil, nil
	}

	if !strings.Contains(rawProxy, "://") {
		rawProxy = "http://" + rawProxy
	}

	proxy, err := neturl.Parse(rawProxy)
	if err != nil {
		// net/url includes the original input, which may contain credentials.
		return nil, errors.New("thttp: proxy URL must be valid")
	}

	switch proxy.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, errors.New("thttp: proxy URL scheme must be http, https, socks5, or socks5h")
	}

	if proxy.Hostname() == "" {
		return nil, errors.New("thttp: proxy URL host must be set")
	}

	return proxy, nil
}

// WithOption sets a client-wide option. Invalid transport options are returned
// by [HttpClient.Do] until a successful transport update clears the error.
func (p *HttpClient) WithOption(key int, value any) *HttpClient {
	return p.WithOptions(map[int]any{key: value})
}

// WithTimeout sets the default total timeout for requests made by this client ([OptTimeout]).
func (p *HttpClient) WithTimeout(timeout time.Duration) *HttpClient {
	return p.WithOption(OptTimeout, timeout)
}

// WithProxyUrl configures a static HTTP proxy address (for example "host:port").
func (p *HttpClient) WithProxyUrl(addr string) *HttpClient {
	return p.WithOption(OptTransProxyUrl, addr)
}

// WithProxyFunc configures a function that supplies proxy information for each request.
func (p *HttpClient) WithProxyFunc(option ProxyFunc) *HttpClient {
	return p.WithOption(OptTransProxyFunc, option)
}

// WithUnsafeTls controls whether TLS certificate verification is skipped for HTTPS requests.
func (p *HttpClient) WithUnsafeTls(unsafe bool) *HttpClient {
	return p.WithOption(OptTransUnsafeTls, unsafe)
}

// WithRetryTransOption enables the retrying [http.RoundTripper] with the supplied configuration.
func (p *HttpClient) WithRetryTransOption(option *RetryTransOption) *HttpClient {
	return p.WithOption(OptTransRetry, option)
}

// WithLogTransOption enables the logging [http.RoundTripper] with the supplied configuration.
func (p *HttpClient) WithLogTransOption(option *LogTransOption) *HttpClient {
	return p.WithOption(OptTransLog, option)
}

// WithCookieJar sets the default cookie jar. Nil disables stored cookies.
func (p *HttpClient) WithCookieJar(jar http.CookieJar) *HttpClient {
	return p.WithOption(OptCookieJar, jar)
}

// WithRedirectPolicy sets [http.Client.CheckRedirect] for requests from this client.
func (p *HttpClient) WithRedirectPolicy(option RedirectPolicyFunc) *HttpClient {
	return p.WithOption(OptRedirectPolicy, option)
}

// WithRequestHookFunc registers a hook invoked before each [http.Client.Do] call.
func (p *HttpClient) WithRequestHookFunc(option RequestHookFunc) *HttpClient {
	return p.WithOption(OptExtraRequestHookFunc, option)
}

// WithResponseHookFunc registers a hook invoked after each [http.Client.Do] call completes.
func (p *HttpClient) WithResponseHookFunc(option ResponseHookFunc) *HttpClient {
	return p.WithOption(OptExtraResponseHookFunc, option)
}

// WithOptions applies a batch of options without publishing a partial transport.
func (p *HttpClient) WithOptions(optionMap map[int]any) *HttpClient {
	p.Lock()
	defer p.Unlock()

	p.applyOptionsLocked(optionMap)

	return p
}

// WithHeader sets a default header for subsequent requests.
func (p *HttpClient) WithHeader(key string, value string) *HttpClient {
	return p.WithHeaders(map[string]string{key: value})
}

// WithReferer sets the default Referer header.
func (p *HttpClient) WithReferer(val string) *HttpClient {
	return p.WithHeader("referer", val)
}

// WithUserAgent sets the default User-Agent header.
func (p *HttpClient) WithUserAgent(val string) *HttpClient {
	return p.WithHeader("user-agent", val)
}

// WithContentType sets the default Content-Type header.
func (p *HttpClient) WithContentType(val string) *HttpClient {
	return p.WithHeader("content-type", val)
}

// WithHeaders merges multiple default headers in a single client update.
func (p *HttpClient) WithHeaders(headerMap map[string]string) *HttpClient {
	p.Lock()
	defer p.Unlock()

	p.mergeHeadersLocked(headerMap)

	return p
}

// mergeHeadersLocked merges case-insensitive headers while the client is locked.
func (p *HttpClient) mergeHeadersLocked(headerMap map[string]string) {
	if len(headerMap) == 0 {
		return
	}

	if p.headerMap == nil {
		p.headerMap = make(map[string]string, len(headerMap))
	}

	for headerName, value := range headerMap {
		p.headerMap[strings.ToLower(headerName)] = value
	}
}

// Do executes a request using a snapshot of client defaults and request options.
// With retries enabled, body replay requires [http.Request.GetBody].
func (p *HttpClient) Do(ctx context.Context, method string, url string, requestOption *RequestOption, body io.Reader) (*Response, error) {
	return p.do(ctx, method, url, requestOption, body, "")
}

// do applies an optional helper-specific content type only to the current request.
func (p *HttpClient) do(ctx context.Context, method string, url string, requestOption *RequestOption, body io.Reader, contentType string) (*Response, error) {
	p.lazyInitTransport()

	snapshot := p.newClientSnapshot()
	if snapshot.transportErr != nil {
		return nil, snapshot.transportErr
	}

	optionMap, headerMap := snapshot.optionMap, snapshot.headerMap
	var cookies []*http.Cookie
	if requestOption != nil {
		requestSnapshot := requestOption.snapshot()
		maps.Copy(optionMap, requestSnapshot.optionMap)
		maps.Copy(headerMap, requestSnapshot.headerMap)
		cookies = requestSnapshot.cookies
	}

	if contentType != "" {
		headerMap["content-type"] = contentType
	}

	transport, err := wrapTransport(snapshot.transport, optionMap)
	if err != nil {
		return nil, err
	}

	cookieJar, err := prepareCookieJar(optionMap, snapshot.cookieJar)
	if err != nil {
		return nil, err
	}

	redirectPolicy, err := prepareRedirect(optionMap)
	if err != nil {
		return nil, err
	}

	timeout := 30 * time.Second
	timeoutValue, ok := optionMap[OptTimeout]
	if ok {
		timeout, ok = timeoutValue.(time.Duration)
		if !ok {
			return nil, fmt.Errorf("thttp: invalid OptTimeout value: want time.Duration, got %T", timeoutValue)
		}
	}

	var requestHook RequestHookFunc
	requestHookValue, ok := optionMap[OptExtraRequestHookFunc]
	if ok {
		requestHook, ok = requestHookValue.(RequestHookFunc)
		if !ok {
			return nil, fmt.Errorf("thttp: invalid OptExtraRequestHookFunc value: want RequestHookFunc, got %T", requestHookValue)
		}
	}

	var responseHook ResponseHookFunc
	responseHookValue, ok := optionMap[OptExtraResponseHookFunc]
	if ok {
		responseHook, ok = responseHookValue.(ResponseHookFunc)
		if !ok {
			return nil, fmt.Errorf("thttp: invalid OptExtraResponseHookFunc value: want ResponseHookFunc, got %T", responseHookValue)
		}
	}

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: redirectPolicy,
		Jar:           cookieJar,
		Timeout:       timeout,
	}
	request, err := prepareRequest(ctx, method, url, headerMap, body)
	if err != nil {
		return nil, err
	}

	if requestHook != nil {
		requestHook(client, request)
	}

	if len(cookies) > 0 && request.URL != nil {
		jar, err := newRequestCookieJar(client.Jar, request, cookies)
		if err != nil {
			if request.Body != nil {
				_ = request.Body.Close()
			}

			return nil, err
		}

		client.Jar = jar
	}

	if snapshot.withDebug {
		event := withRequestDetails(tlog.I(request.Context()), request, nil)
		addHeaderNames(event, "req", request.Header)
		event.Msg("thttp outbound request metadata")
	}

	response, err := client.Do(request)
	if err != nil || (response != nil && response.StatusCode >= http.StatusBadRequest) {
		event := withRequestDetails(tlog.W(request.Context()), request, err)
		if response != nil {
			event.Detailf("resp.status code: %d", response.StatusCode)
		}

		event.Msg("thttp request failed or returned HTTP status >= 400")
	}

	if responseHook != nil {
		responseHook(response, err)
	}

	if response == nil {
		return nil, err
	}

	responseData := &Response{Response: response}

	return responseData, err
}

// send converts params to an [io.Reader] for the given verb helpers and calls [HttpClient.Do].
// Supported types for params are listed in the error returned from the default branch.
func (p *HttpClient) send(ctx context.Context, method string, url string, requestOption *RequestOption, params any) (*Response, error) {
	var body io.Reader

	switch retParams := params.(type) {
	case nil:
		body = bytes.NewReader(nil)
	case []byte:
		body = bytes.NewReader(retParams)
	case string:
		body = strings.NewReader(retParams)
	case *bytes.Reader:
		body = retParams
	case neturl.Values:
		body = strings.NewReader(retParams.Encode())
	default:
		return nil, fmt.Errorf("thttp: unsupported request body type %T; supported types: nil, []byte, string, *bytes.Reader, url.Values", retParams)
	}

	return p.Do(ctx, method, url, requestOption, body)
}

// sendJson serializes params to JSON when needed and sets Content-Type to [ContentTypeApplicationJson].
func (p *HttpClient) sendJson(ctx context.Context, method string, url string, requestOption *RequestOption, params any) (*Response, error) {
	var body io.Reader

	switch retParams := params.(type) {
	case nil:
		body = bytes.NewReader(nil)
	case []byte:
		body = bytes.NewReader(retParams)
	case string:
		body = strings.NewReader(retParams)
	case *bytes.Reader:
		body = retParams
	default:
		data, err := json.Marshal(retParams)
		if err != nil {
			return nil, err
		}

		body = bytes.NewReader(data)
	}

	return p.do(ctx, method, url, requestOption, body, ContentTypeApplicationJson)
}

// Head sends an HTTP HEAD request.
func (p *HttpClient) Head(ctx context.Context, url string, requestOption *RequestOption) (*Response, error) {
	return p.Do(ctx, http.MethodHead, url, requestOption, nil)
}

// Get sends an HTTP GET request, appending params as the query string.
func (p *HttpClient) Get(ctx context.Context, url string, requestOption *RequestOption, params neturl.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.Do(ctx, http.MethodGet, url, requestOption, nil)
}

// GetLen performs a HEAD request and returns the Content-Length value when present and parseable.
func (p *HttpClient) GetLen(ctx context.Context, url string, requestOption *RequestOption, params neturl.Values) (int64, error) {
	url = appendParams(url, params)

	resp, err := p.Do(ctx, http.MethodHead, url, requestOption, nil)
	if resp != nil && resp.Response != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		return -1, err
	}

	contentLen := resp.Header.Get("Content-Length")

	length, err := strconv.ParseInt(contentLen, 10, 64)
	if err != nil {
		return -1, err
	}

	return length, nil
}

// Post sends an HTTP POST request. The body is supplied as a byte slice via [HttpClient.send].
func (p *HttpClient) Post(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return p.send(ctx, http.MethodPost, url, requestOption, params)
}

// PostJson sends an HTTP POST with a JSON-encoded body (structs and maps are marshaled with [encoding/json.Marshal]).
func (p *HttpClient) PostJson(ctx context.Context, url string, requestOption *RequestOption, params any) (*Response, error) {
	return p.sendJson(ctx, http.MethodPost, url, requestOption, params)
}

// PostMultipart sends multipart/form-data. Field names prefixed with '@' are treated as file paths for upload.
func (p *HttpClient) PostMultipart(ctx context.Context, url string, requestOption *RequestOption, params neturl.Values) (*Response, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, values := range params {
		for _, value := range values {
			if err := writeMultipartField(writer, key, value); err != nil {
				return nil, err
			}
		}
	}

	return p.sendMultipart(ctx, url, requestOption, body, writer)
}

// FormData is a single multipart field name and value pair.
type FormData struct {
	Key   string
	Value string
}

// PostMultipartEx sends multipart form data using an explicit slice of [FormData].
func (p *HttpClient) PostMultipartEx(ctx context.Context, url string, requestOption *RequestOption, params []*FormData) (*Response, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for index, formData := range params {
		if formData == nil {
			return nil, fmt.Errorf("thttp: multipart field at index %d must not be nil", index)
		}

		if err := writeMultipartField(writer, formData.Key, formData.Value); err != nil {
			return nil, err
		}
	}

	return p.sendMultipart(ctx, url, requestOption, body, writer)
}

// writeMultipartField treats names starting with '@' as file fields.
func writeMultipartField(writer *multipart.Writer, key string, value string) error {
	fieldName, isFile := strings.CutPrefix(key, "@")
	if isFile {
		return loadFormFile(writer, fieldName, value)
	}

	return writer.WriteField(fieldName, value)
}

// sendMultipart finishes the body before issuing an HTTP request.
func (p *HttpClient) sendMultipart(ctx context.Context, url string, requestOption *RequestOption, body *bytes.Buffer, writer *multipart.Writer) (*Response, error) {
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("thttp: finish multipart body: %w", err)
	}

	return p.do(ctx, http.MethodPost, url, requestOption, body, writer.FormDataContentType())
}

// Put sends an HTTP PUT request with a raw body.
func (p *HttpClient) Put(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return p.send(ctx, http.MethodPut, url, requestOption, params)
}

// PutJson sends an HTTP PUT with a JSON-encoded body.
func (p *HttpClient) PutJson(ctx context.Context, url string, requestOption *RequestOption, params any) (*Response, error) {
	return p.sendJson(ctx, http.MethodPut, url, requestOption, params)
}

// Patch sends an HTTP PATCH request with a raw body.
func (p *HttpClient) Patch(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return p.send(ctx, http.MethodPatch, url, requestOption, params)
}

// PatchJson sends an HTTP PATCH with a JSON-encoded body.
func (p *HttpClient) PatchJson(ctx context.Context, url string, requestOption *RequestOption, params any) (*Response, error) {
	return p.sendJson(ctx, http.MethodPatch, url, requestOption, params)
}

// Delete sends an HTTP DELETE request, appending params as the query string.
func (p *HttpClient) Delete(ctx context.Context, url string, requestOption *RequestOption, params neturl.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, http.MethodDelete, url, requestOption, nil)
}

// Options sends an HTTP OPTIONS request.
func (p *HttpClient) Options(ctx context.Context, url string, requestOption *RequestOption, params neturl.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, http.MethodOptions, url, requestOption, nil)
}

// Connect sends an HTTP CONNECT request.
func (p *HttpClient) Connect(ctx context.Context, url string, requestOption *RequestOption, params neturl.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, http.MethodConnect, url, requestOption, nil)
}

// Trace sends an HTTP TRACE request.
func (p *HttpClient) Trace(ctx context.Context, url string, requestOption *RequestOption, params neturl.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, http.MethodTrace, url, requestOption, nil)
}
