package thttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	_url "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/choveylee/tlog"
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

// OptTransports lists option keys that update the shared [http.Transport] via [HttpClient.WithOption].
// Options not in this set are stored and applied when building each [http.Client] in [HttpClient.Do].
var (
	OptTransports = map[int]int{
		OptTransProxyUrl:  OptTransProxyUrl,
		OptTransProxyFunc: OptTransProxyFunc,

		OptTransMaxIdleConns:        OptTransMaxIdleConns,
		OptTransMaxIdleConnsPerHost: OptTransMaxIdleConnsPerHost,
		OptTransMaxConnsPerHost:     OptTransMaxConnsPerHost,

		OptTransUnsafeTls: OptTransUnsafeTls,
		OptTransTlsConfig: OptTransTlsConfig,
	}
)

// newDefaultTransport returns an [http.Transport] with dial timeouts, HTTP/2, and idle connection defaults.
func newDefaultTransport() *http.Transport {
	return &http.Transport{
		DialContext: defaultTransportDialContext(&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}),

		ForceAttemptHTTP2: true,

		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,

		TLSHandshakeTimeout: 10 * time.Second,

		ExpectContinueTimeout: 1 * time.Second,
	}
}

// ProxyFunc is the same type as [http.Transport.Proxy] (alias so values round-trip through [HttpClient.WithOption]).
type ProxyFunc = func(*http.Request) (*_url.URL, error)

// RedirectPolicyFunc is the same type as [http.Client.CheckRedirect].
type RedirectPolicyFunc = func(*http.Request, []*http.Request) error

// RequestHookFunc is called immediately before [http.Client.Do] executes the request (alias so hooks survive [HttpClient.WithOption]).
type RequestHookFunc = func(*http.Client, *http.Request)

// ResponseHookFunc is called after [http.Client.Do] returns, receiving the response and error from the round trip.
type ResponseHookFunc = func(*http.Response, error)

// prepareRequest builds an [http.Request] with the given method, URL, body, and default headers.
// It does not set [http.Request.GetBody]; callers that require retriable bodies must assign it separately.
func prepareRequest(ctx context.Context, method string, url string, headers map[string]string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}

	return req, nil
}

// snapshotRequestBody reopens and reads the request body for logging when [http.Request.GetBody] is available.
func snapshotRequestBody(req *http.Request) []byte {
	if req == nil || req.GetBody == nil {
		return nil
	}

	body, err := req.GetBody()
	if err != nil {
		return nil
	}

	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil
	}

	return data
}

// dumpDebugRequest builds a request snapshot for debug logging after hooks and cookie resolution.
// When [http.Request.GetBody] is unavailable, the dump omits the body to avoid consuming the live stream.
func dumpDebugRequest(req *http.Request, cookieJar http.CookieJar) []byte {
	if req == nil {
		return nil
	}

	debugReq := req.Clone(req.Context())
	debugReq.Header = req.Header.Clone()

	dumpBody := false
	if req.Body != nil && req.Body != http.NoBody && req.GetBody != nil {
		body, err := req.GetBody()
		if err == nil {
			debugReq.Body = body
			defer debugReq.Body.Close()
			dumpBody = true
		} else {
			debugReq.Body = nil
		}
	} else {
		debugReq.Body = nil
	}

	if cookieJar != nil && debugReq.URL != nil {
		for _, cookie := range cookieJar.Cookies(debugReq.URL) {
			debugReq.AddCookie(cookie)
		}
	}

	dump, err := httputil.DumpRequestOut(debugReq, dumpBody)
	if err != nil {
		return nil
	}

	return dump
}

type clientSnapshot struct {
	options map[int]interface{}
	headers map[string]string

	transport *http.Transport

	cookieJar http.CookieJar

	transportErr error

	withDebug bool
}

// defaultTransportDialContext adapts a [net.Dialer] for use as [http.Transport.DialContext].
func defaultTransportDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return dialer.DialContext
}

// wrapTransport decorates transport with [OptTransLog] first, then [OptTransRetry] (retry is the outermost [http.RoundTripper]).
// Invalid option value types return errors prefixed with "thttp:" and use "invalid <option> value" wording.
func wrapTransport(transport http.RoundTripper, options map[int]interface{}) (http.RoundTripper, error) {
	// add log transport
	logTransOption := defaultLogTransOption

	srcLogTransOption, ok := options[OptTransLog]
	if ok == true {
		destLogTransOption, ok := srcLogTransOption.(*LogTransOption)
		if ok == true {
			logTransOption = destLogTransOption
		} else {
			return nil, fmt.Errorf("thttp: invalid OptTransLog value: want *LogTransOption, got %T", srcLogTransOption)
		}
	}

	desTransport := wrapLogTransport(transport, logTransOption)

	// add retry transport
	srcRetryTransOption, ok := options[OptTransRetry]
	if ok == true {
		desRetryTransOption, ok := srcRetryTransOption.(*RetryTransOption)
		if ok == true {
			desTransport = wrapRetryTransport(desTransport, desRetryTransOption)
		} else {
			return nil, fmt.Errorf("thttp: invalid OptTransRetry value: want *RetryTransOption, got %T", srcRetryTransOption)
		}
	}

	return desTransport, nil
}

// prepareCookieJar interprets [OptCookieJar]: absent or true reuses defaultJar when available (otherwise allocates a
// new jar), [http.CookieJar] is used as-is, and false disables cookies for the request. Any other type returns an
// error using "invalid OptCookieJar value" wording.
func prepareCookieJar(options map[int]interface{}, defaultJar http.CookieJar) (http.CookieJar, error) {
	srcOptCookieJar, ok := options[OptCookieJar]
	if ok == false {
		if defaultJar != nil {
			return defaultJar, nil
		}

		return nil, nil
	}

	optCookieJar, ok := srcOptCookieJar.(bool)
	if ok == true {
		if optCookieJar == false {
			return nil, nil
		}

		if defaultJar != nil {
			return defaultJar, nil
		}

		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, err
		}

		return jar, nil
	}

	jar, ok := srcOptCookieJar.(http.CookieJar)
	if ok == false {
		return nil, fmt.Errorf("thttp: invalid OptCookieJar value: want bool or http.CookieJar, got %T", srcOptCookieJar)
	}

	return jar, nil
}

// prepareRedirect returns the redirect handler from [OptRedirectPolicy] when set, or nil if unset.
func prepareRedirect(options map[int]interface{}) (func(req *http.Request, via []*http.Request) error, error) {
	var redirectPolicy func(req *http.Request, via []*http.Request) error

	srcRedirectPolicy, ok := options[OptRedirectPolicy]
	if ok == true {
		destRedirectPolicy, ok := srcRedirectPolicy.(func(*http.Request, []*http.Request) error)
		if ok == false {
			return nil, fmt.Errorf("thttp: invalid OptRedirectPolicy value: want func(*http.Request, []*http.Request) error, got %T", srcRedirectPolicy)
		}

		redirectPolicy = destRedirectPolicy
	}

	return redirectPolicy, nil
}

// HttpClient holds default transport settings, per-client options, and default headers used by [HttpClient.Do]
// and the HTTP verb helpers.
type HttpClient struct {
	// options stores [OptTimeout], hook functions, logging and retry configuration, and other non-transport keys.
	options map[int]interface{}

	// headers holds default header keys (lowercased) merged into every request unless overridden per call.
	headers map[string]string

	// transport is the base [http.Transport] mutated by transport-related options.
	transport *http.Transport

	connectTimeout  time.Duration
	deadlineTimeout time.Duration

	// cookieJar is the optional default jar used when constructing clients if not overridden per request.
	cookieJar http.CookieJar

	// withDebug enables dumping of the outbound request via [net/http/httputil.DumpRequestOut].
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

	options := make(map[int]interface{}, len(p.options))
	for key, val := range p.options {
		options[key] = val
	}

	headers := make(map[string]string, len(p.headers))
	for key, val := range p.headers {
		headers[key] = val
	}

	return clientSnapshot{
		options: options,
		headers: headers,

		transport: p.transport,

		cookieJar: p.cookieJar,

		transportErr: p.transportErr,

		withDebug: p.withDebug,
	}
}

// NewHttpClient returns an [HttpClient] with a fresh transport, 30-second connect and deadline timeouts,
// and a default cookie jar when [cookiejar.New] succeeds.
// A zero [HttpClient] is usable: the first [HttpClient.Do] or [HttpClient.Transport] runs [HttpClient.lazyInitTransport]
// to initialize the transport and default cookie jar; transport [HttpClient.WithOption] / [HttpClient.Defaults] use
// [HttpClient.ensureTransportLocked] under [HttpClient.Lock].
func NewHttpClient() *HttpClient {
	httpClient := &HttpClient{
		options: make(map[int]interface{}),
		headers: make(map[string]string),

		transport: newDefaultTransport(),

		connectTimeout:  30 * time.Second,
		deadlineTimeout: 30 * time.Second,
	}

	cookieJar, err := cookiejar.New(nil)
	if err == nil {
		httpClient.cookieJar = cookieJar
	}

	return httpClient
}

// cloneOptionValue returns a shallow copy for [*LogTransOption] and [*RetryTransOption] so stored config is not
// aliased to a template the caller may mutate from another goroutine.
func cloneOptionValue(key int, val interface{}) interface{} {
	switch key {
	case OptTransLog:
		logTransOption, ok := val.(*LogTransOption)
		if !ok {
			return val
		}

		if logTransOption == nil {
			return nil
		}

		copied := *logTransOption

		return &copied
	case OptTransRetry:
		retryTransOption, ok := val.(*RetryTransOption)
		if !ok {
			return val
		}

		if retryTransOption == nil {
			return nil
		}

		copied := *retryTransOption

		return &copied
	default:
		return val
	}
}

// Defaults merges the provided options and headers into the client defaults. Keys in [OptTransports] are applied
// to the shared [http.Transport] (and are not stored in the options map), matching [HttpClient.WithOption].
// [LogTransOption] and [RetryTransOption] values are copied when stored so later mutation of the caller's struct
// does not affect this client. It is not safe for concurrent use with in-flight requests unless the caller synchronizes access.
func (p *HttpClient) Defaults(options map[int]interface{}, headers map[string]string) *HttpClient {
	p.Lock()
	defer p.Unlock()

	var transportErr error

	transportChanged := false

	for key, val := range options {
		_, ok := OptTransports[key]
		if ok {
			transportChanged = true

			if p.options != nil {
				delete(p.options, key)
			}

			err := p.resetTransport(key, val)
			if err != nil {
				transportErr = errors.Join(transportErr, err)

				tlog.E(context.Background()).Err(err).Msgf("thttp transport option update failed: %v", err)
			}

			continue
		}

		if p.options == nil {
			p.options = make(map[int]interface{})
		}

		p.options[key] = cloneOptionValue(key, val)
	}

	if len(headers) > 0 {
		if p.headers == nil {
			p.headers = make(map[string]string)
		}

		for key, val := range headers {
			p.headers[strings.ToLower(key)] = val
		}
	}

	if transportErr != nil {
		p.transportErr = transportErr
	} else if transportChanged {
		p.transportErr = nil
	}

	return p
}

// Debug enables or disables verbose request logging for this client.
func (p *HttpClient) Debug(val bool) *HttpClient {
	p.Lock()
	defer p.Unlock()

	p.withDebug = val

	return p
}

// lazyInitTransport allocates [HttpClient.transport] once on first [HttpClient.Do] or [HttpClient.Transport].
// [HttpClient.WithOption] transport keys use [HttpClient.ensureTransportLocked] instead (caller already holds [HttpClient.Lock]).
func (p *HttpClient) lazyInitTransport() {
	p.transportInit.Do(func() {
		p.Lock()
		defer p.Unlock()

		if p.transport == nil {
			p.transport = newDefaultTransport()
		}

		if p.cookieJar == nil {
			cookieJar, err := cookiejar.New(nil)
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

// ensureTransportLocked sets [HttpClient.transport] if nil. Caller must hold [HttpClient.Lock].
func (p *HttpClient) ensureTransportLocked() {
	if p.transport == nil {
		p.transport = newDefaultTransport()
	}
}

// resetTransport applies one [OptTransports] option to [HttpClient.transport]. The caller must hold [HttpClient.Lock].
// It returns an error if val is not assignable to the documented Go type for that option key.
func (p *HttpClient) resetTransport(key int, val interface{}) error {
	p.ensureTransportLocked()

	transport := p.transport.Clone()

	if key == OptTransMaxIdleConns {
		destMaxIdleConns, ok := val.(int)
		if ok == true {
			transport.MaxIdleConns = destMaxIdleConns
			p.transport = transport

			return nil
		}

		return fmt.Errorf("thttp: invalid OptTransMaxIdleConns value: want int, got %T", val)
	}

	if key == OptTransMaxIdleConnsPerHost {
		destMaxIdleConnsPerHost, ok := val.(int)
		if ok == true {
			transport.MaxIdleConnsPerHost = destMaxIdleConnsPerHost
			p.transport = transport

			return nil
		}

		return fmt.Errorf("thttp: invalid OptTransMaxIdleConnsPerHost value: want int, got %T", val)
	}

	if key == OptTransMaxConnsPerHost {
		destMaxConnsPerHost, ok := val.(int)
		if ok == true {
			transport.MaxConnsPerHost = destMaxConnsPerHost
			p.transport = transport

			return nil
		}

		return fmt.Errorf("thttp: invalid OptTransMaxConnsPerHost value: want int, got %T", val)
	}

	// proxy
	if key == OptTransProxyFunc {
		destProxyFunc, ok := val.(ProxyFunc)
		if ok == true {
			transport.Proxy = destProxyFunc
			p.transport = transport

			return nil
		}

		return fmt.Errorf("thttp: invalid OptTransProxyFunc value: want ProxyFunc (func(*http.Request) (*url.URL, error)), got %T", val)
	} else if key == OptTransProxyUrl {
		destProxy, ok := val.(string)
		if ok == true {
			proxy := destProxy

			if strings.Contains(proxy, "://") == false {
				proxy = "http://" + proxy
			}

			proxyUrl, err := _url.Parse(proxy)
			if err != nil {
				return err
			}

			transport.Proxy = http.ProxyURL(proxyUrl)
			p.transport = transport

			return nil
		}

		return fmt.Errorf("thttp: invalid OptTransProxyUrl value: want string, got %T", val)
	}

	if key == OptTransUnsafeTls {
		destUnsafeTls, ok := val.(bool)
		if ok == true {
			unsafeTls := destUnsafeTls

			tlsConfig := transport.TLSClientConfig

			if tlsConfig == nil {
				tlsConfig = &tls.Config{}
				transport.TLSClientConfig = tlsConfig
			}

			tlsConfig.InsecureSkipVerify = unsafeTls
			p.transport = transport

			return nil
		}

		return fmt.Errorf("thttp: invalid OptTransUnsafeTls value: want bool, got %T", val)
	}

	if key == OptTransTlsConfig {
		destTlsConfig, ok := val.(*tls.Config)
		if ok == true {
			transport.TLSClientConfig = destTlsConfig
			p.transport = transport
		} else {
			return fmt.Errorf("thttp: invalid OptTransTlsConfig value: want *tls.Config, got %T", val)
		}
	}

	return nil
}

// WithOption sets a client-wide option. Keys listed in [OptTransports] update the shared [http.Transport];
// other keys are stored for use when [HttpClient.Do] builds the [http.Client].
// [*LogTransOption] and [*RetryTransOption] are shallow-copied when stored so callers can reuse the same template safely.
// A failed transport update is logged and causes subsequent [HttpClient.Do] calls to return that error until a transport option applies successfully ([HttpClient.Defaults] behaves the same for transport keys).
func (p *HttpClient) WithOption(key int, val interface{}) *HttpClient {
	p.Lock()
	defer p.Unlock()

	_, ok := OptTransports[key]
	if ok == true {
		err := p.resetTransport(key, val)
		if err != nil {
			p.transportErr = err

			tlog.E(context.Background()).Err(err).Msgf("thttp transport option update failed: %v",
				err)
		} else {
			p.transportErr = nil
		}
	} else {
		if p.options == nil {
			p.options = make(map[int]interface{})
		}

		p.options[key] = cloneOptionValue(key, val)
	}

	return p
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

// WithCookieJar sets the default [http.CookieJar] for this client ([OptCookieJar]).
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

// WithOptions applies multiple options in sequence.
func (p *HttpClient) WithOptions(options map[int]interface{}) *HttpClient {
	p.Lock()
	defer p.Unlock()

	var transportErr error

	transportChanged := false

	for key, val := range options {
		_, ok := OptTransports[key]
		if ok {
			transportChanged = true

			err := p.resetTransport(key, val)
			if err != nil {
				transportErr = errors.Join(transportErr, err)

				tlog.E(context.Background()).Err(err).Msgf("thttp transport option update failed: %v",
					err)
			}

			continue
		}

		if p.options == nil {
			p.options = make(map[int]interface{})
		}

		p.options[key] = cloneOptionValue(key, val)
	}

	if transportErr != nil {
		p.transportErr = transportErr
	} else if transportChanged {
		p.transportErr = nil
	}

	return p
}

// WithHeader sets a default header for all subsequent requests (key is stored in lowercase).
func (p *HttpClient) WithHeader(key string, val string) *HttpClient {
	p.Lock()
	defer p.Unlock()

	if p.headers == nil {
		p.headers = make(map[string]string)
	}

	p.headers[strings.ToLower(key)] = val

	return p
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

// WithHeaders merges multiple default headers.
func (p *HttpClient) WithHeaders(headers map[string]string) *HttpClient {
	for key, val := range headers {
		p.WithHeader(key, val)
	}

	return p
}

// Do executes an HTTP request: it merges client defaults with requestOption, builds an [http.Client] with
// wrapped transport (logging, then retry), and returns a [Response]. Request bodies are passed through as-is;
// replay only happens when [http.Request.GetBody] is already available or the retry layer chooses to buffer.
func (p *HttpClient) Do(ctx context.Context, method string, url string, requestOption *RequestOption, body io.Reader) (*Response, error) {
	p.lazyInitTransport()

	snapshot := p.newClientSnapshot()
	if snapshot.transportErr != nil {
		return nil, snapshot.transportErr
	}

	options := snapshot.options
	headers := snapshot.headers
	cookies := make([]*http.Cookie, 0)

	if requestOption != nil {
		requestSnapshot := requestOption.snapshot()

		for key, val := range requestSnapshot.options {
			options[key] = val
		}

		for key, val := range requestSnapshot.headers {
			headers[key] = val
		}

		cookies = requestSnapshot.cookies
	}

	transport, err := wrapTransport(snapshot.transport, options)
	if err != nil {
		return nil, err
	}

	cookieJar, err := prepareCookieJar(options, snapshot.cookieJar)
	if err != nil {
		return nil, err
	}

	redirect, err := prepareRedirect(options)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: redirect,
		Jar:           cookieJar,
	}

	timeout := time.Second * 30

	srcTimeout, ok := options[OptTimeout]
	if ok == true {
		destTimeout, ok := srcTimeout.(time.Duration)
		if ok == false {
			return nil, fmt.Errorf("thttp: invalid OptTimeout value: want time.Duration, got %T", srcTimeout)
		}

		timeout = destTimeout
	}

	client.Timeout = timeout

	request, err := prepareRequest(ctx, method, url, headers, body)
	if err != nil {
		return nil, err
	}

	if cookieJar != nil {
		cookieJar.SetCookies(request.URL, cookies)
	} else {
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
	}

	srcRequestHookFunc, ok := options[OptExtraRequestHookFunc]
	if ok == true {
		requestHookFunc, ok := srcRequestHookFunc.(RequestHookFunc)
		if ok == true {
			requestHookFunc(client, request)
		}
	}

	if snapshot.withDebug == true {
		dump := dumpDebugRequest(request, cookieJar)
		if dump != nil {
			tlog.I(ctx).Msgf("thttp outbound request dump:\n%s", dump)
		}
	}

	isAbnormal := false

	response, err := client.Do(request)
	if err != nil {
		isAbnormal = true
	} else if response != nil && response.StatusCode >= 400 {
		isAbnormal = true
	}

	if isAbnormal {
		event := tlog.W(request.Context()).Err(err).Detailf("req.method: %s", request.Method).
			Detailf("req.host: %s", request.Host).Detailf("req.url: %s", request.URL.String())

		bodyBytes := snapshotRequestBody(request)
		if bodyBytes != nil {
			event = event.Detailf("req.body: %s", string(bodyBytes))
		}

		for key, vals := range request.Header {
			event = event.Detailf("req.header.%s: %s", key, strings.Join(vals, ";"))
		}

		if response != nil {
			event = event.Detailf("resp.status code: %d", response.StatusCode)
		}

		event.Msg("thttp request failed or returned HTTP status >= 400")
	}

	srcResponseHookFunc, ok := options[OptExtraResponseHookFunc]
	if ok == true {
		responseHookFunc, ok := srcResponseHookFunc.(ResponseHookFunc)
		if ok == true {
			responseHookFunc(response, err)
		}
	}

	if response == nil {
		return nil, err
	}

	return &Response{response}, err
}

// send converts params to an [io.Reader] for the given verb helpers and calls [HttpClient.Do].
// Supported types for params are listed in the error returned from the default branch.
func (p *HttpClient) send(ctx context.Context, method string, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
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
	case _url.Values:
		body = strings.NewReader(retParams.Encode())
	default:
		return nil, fmt.Errorf("thttp: unsupported request body type %T; supported types: nil, []byte, string, *bytes.Reader, url.Values", retParams)
	}

	return p.Do(ctx, method, url, requestOption, body)
}

// sendJson serializes params to JSON when needed and sets Content-Type to [ContentTypeApplicationJson].
func (p *HttpClient) sendJson(ctx context.Context, method string, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	if requestOption == nil {
		requestOption = NewRequestOption()
	}

	requestOption.WithContentType(ContentTypeApplicationJson)

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

	return p.Do(ctx, method, url, requestOption, body)
}

// Head sends an HTTP HEAD request.
func (p *HttpClient) Head(ctx context.Context, url string, requestOption *RequestOption) (*Response, error) {
	return p.Do(ctx, "HEAD", url, requestOption, nil)
}

// Get sends an HTTP GET request, appending params as the query string.
func (p *HttpClient) Get(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.Do(ctx, "GET", url, requestOption, nil)
}

// GetLen performs a HEAD request and returns the Content-Length value when present and parseable.
func (p *HttpClient) GetLen(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (int64, error) {
	url = appendParams(url, params)

	resp, err := p.Do(ctx, "HEAD", url, requestOption, nil)
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
	/*
		switch retParams := params.(type) {
		case _url.Values:
			for key := range retParams {
				if len(key) > 0 && key[0] == '@' {
					return p.PostMultipart(ctx, url, retParams)
				}
			}
		}
	*/

	return p.send(ctx, "POST", url, requestOption, params)
}

// PostJson sends an HTTP POST with a JSON-encoded body (structs and maps are marshaled with [encoding/json.Marshal]).
func (p *HttpClient) PostJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return p.sendJson(ctx, "POST", url, requestOption, params)
}

// PostMultipart sends multipart/form-data. Field names prefixed with '@' are treated as file paths for upload.
func (p *HttpClient) PostMultipart(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for key, values := range params {
		for _, value := range values {
			// if value is file
			if len(key) > 0 && key[0] == '@' {
				err := loadFormFile(writer, key[1:], value)
				if err != nil {
					return nil, err
				}
			} else {
				err := writer.WriteField(key, value)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	if requestOption == nil {
		requestOption = NewRequestOption()
	}

	requestOption.WithContentType(writer.FormDataContentType())

	err := writer.Close()
	if err != nil {
		return nil, err
	}

	return p.Do(ctx, "POST", url, requestOption, body)
}

// FormData is a single multipart field name and value pair.
type FormData struct {
	Key   string
	Value string
}

// PostMultipartEx sends multipart/form-data using an explicit slice of [FormData] fields.
func (p *HttpClient) PostMultipartEx(ctx context.Context, url string, requestOption *RequestOption, params []*FormData) (*Response, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	defer func() { _ = writer.Close() }()

	for _, formData := range params {
		key := formData.Key
		value := formData.Value

		// if value is file
		if len(key) > 0 && key[0] == '@' {
			err := loadFormFile(writer, key[1:], value)
			if err != nil {
				return nil, err
			}
		} else {
			err := writer.WriteField(key, value)
			if err != nil {
				return nil, err
			}
		}
	}

	if requestOption == nil {
		requestOption = NewRequestOption()
	}

	requestOption.WithContentType(writer.FormDataContentType())

	err := writer.Close()
	if err != nil {
		return nil, err
	}

	return p.Do(ctx, "POST", url, requestOption, body)
}

// Put sends an HTTP PUT request with a raw body.
func (p *HttpClient) Put(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return p.send(ctx, "PUT", url, requestOption, params)
}

// PutJson sends an HTTP PUT with a JSON-encoded body.
func (p *HttpClient) PutJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return p.sendJson(ctx, "PUT", url, requestOption, params)
}

// Patch sends an HTTP PATCH request with a raw body.
func (p *HttpClient) Patch(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return p.send(ctx, "PATCH", url, requestOption, params)
}

// PatchJson sends an HTTP PATCH with a JSON-encoded body.
func (p *HttpClient) PatchJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return p.sendJson(ctx, "PATCH", url, requestOption, params)
}

// Delete sends an HTTP DELETE request, appending params as the query string.
func (p *HttpClient) Delete(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, "DELETE", url, requestOption, nil)
}

// Options sends an HTTP OPTIONS request.
func (p *HttpClient) Options(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, "OPTIONS", url, requestOption, nil)
}

// Connect sends an HTTP CONNECT request.
func (p *HttpClient) Connect(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, "CONNECT", url, requestOption, nil)
}

// Trace sends an HTTP TRACE request.
func (p *HttpClient) Trace(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, "TRACE", url, requestOption, nil)
}
