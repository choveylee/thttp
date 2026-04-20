package thttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
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

// RedirectPolicyFunc is the same shape as [http.Client.CheckRedirect].
type RedirectPolicyFunc func(*http.Request, []*http.Request) error

// RequestHookFunc is called immediately before [http.Client.Do] executes the request (alias so hooks survive [HttpClient.WithOption]).
type RequestHookFunc = func(*http.Client, *http.Request)

// ResponseHookFunc is called after [http.Client.Do] returns, receiving the response and error from the round trip.
type ResponseHookFunc = func(*http.Response, error)

// prepareRequest constructs an [http.Request] with the supplied headers.
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

// defaultTransportDialContext adapts a [net.Dialer] for use as [http.Transport.DialContext].
func defaultTransportDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return dialer.DialContext
}

// wrapTransport layers logging and retry transports according to options.
func wrapTransport(transport http.RoundTripper, options map[int]interface{}) (http.RoundTripper, error) {
	// add log transport
	logTransOption := defaultLogTransOption

	srcLogTransOption, ok := options[OptTransLog]
	if ok == true {
		destLogTransOption, ok := srcLogTransOption.(*LogTransOption)
		if ok == true {
			logTransOption = destLogTransOption
		} else {
			return nil, fmt.Errorf("log trans option type illegal")
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
			return nil, fmt.Errorf("retry trans option type illegal")
		}
	}

	return desTransport, nil
}

// prepareCookieJar resolves [OptCookieJar] into a concrete [http.CookieJar] or nil.
func prepareCookieJar(options map[int]interface{}) (http.CookieJar, error) {
	srcOptCookieJar, ok := options[OptCookieJar]
	if ok == true {
		optCookieJar, ok := srcOptCookieJar.(bool)
		if ok == true {
			// default cookieJar
			if optCookieJar == true {
				// TODO: PublicSuffixList
				jar, err := cookiejar.New(nil)
				if err != nil {
					return nil, err
				}

				return jar, nil
			}
		} else {
			jar, ok := srcOptCookieJar.(http.CookieJar)
			if ok == false {
				return nil, fmt.Errorf("cookie cookieJar type illegal, cookie cookieJar supported")
			}

			return jar, nil
		}
	}

	return nil, nil
}

// prepareRedirect returns [http.Client.CheckRedirect] from [OptRedirectPolicy] when set.
func prepareRedirect(options map[int]interface{}) (func(req *http.Request, via []*http.Request) error, error) {
	var redirectPolicy func(req *http.Request, via []*http.Request) error

	srcRedirectPolicy, ok := options[OptRedirectPolicy]
	if ok == true {
		destRedirectPolicy, ok := srcRedirectPolicy.(func(*http.Request, []*http.Request) error)
		if ok == false {
			return nil, fmt.Errorf("redirect policy type illegal")
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

	sync.RWMutex
}

// NewHttpClient returns an [HttpClient] with a fresh transport, 30-second connect and deadline timeouts,
// and a default cookie jar when [cookiejar.New] succeeds.
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

	for key, val := range options {
		if _, ok := OptTransports[key]; ok {
			if p.options != nil {
				delete(p.options, key)
			}

			err := p.resetTransport(key, val)
			if err != nil {
				p.transportErr = err
				tlog.E(context.Background()).Err(err).Msgf("reset transport err (%v).", err)
			} else {
				p.transportErr = nil
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
			p.headers[key] = val
		}
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

// Transport returns the underlying [http.Transport] used by this client.
func (p *HttpClient) Transport() http.RoundTripper {
	p.Lock()
	defer p.Unlock()

	return p.transport
}

// resetTransport applies a single [OptTransports] key to the shared [http.Transport].
func (p *HttpClient) resetTransport(key int, val interface{}) error {
	if key == OptTransMaxIdleConns {
		destMaxIdleConns, ok := val.(int)
		if ok == true {
			p.transport.MaxIdleConns = destMaxIdleConns

			return nil
		} else {
			return fmt.Errorf("max idle conns type illegal, int supported")
		}
	}

	if key == OptTransMaxIdleConnsPerHost {
		destMaxIdleConnsPerHost, ok := val.(int)
		if ok == true {
			p.transport.MaxIdleConnsPerHost = destMaxIdleConnsPerHost

			return nil
		} else {
			return fmt.Errorf("max idle conns per host type illegal, int supported")
		}
	}

	if key == OptTransMaxConnsPerHost {
		destMaxConnsPerHost, ok := val.(int)
		if ok == true {
			p.transport.MaxConnsPerHost = destMaxConnsPerHost

			return nil
		} else {
			return fmt.Errorf("max conns per host type illegal, int supported")
		}
	}

	// proxy
	if key == OptTransProxyFunc {
		destProxyFunc, ok := val.(ProxyFunc)
		if ok == true {
			p.transport.Proxy = destProxyFunc

			return nil
		}

		return fmt.Errorf("proxy func type illegal, ProxyFunc supported")
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

			p.transport.Proxy = http.ProxyURL(proxyUrl)

			return nil
		} else {
			return fmt.Errorf("proxy type illegal, string supported")
		}
	}

	if key == OptTransUnsafeTls {
		destUnsafeTls, ok := val.(bool)
		if ok == true {
			unsafeTls := destUnsafeTls

			tlsConfig := p.transport.TLSClientConfig

			if tlsConfig == nil {
				tlsConfig = &tls.Config{}
				p.transport.TLSClientConfig = tlsConfig
			}

			tlsConfig.InsecureSkipVerify = unsafeTls

			return nil
		} else {
			return fmt.Errorf("unsafe tls type illegal, bool supported")
		}
	}

	if key == OptTransTlsConfig {
		destTlsConfig, ok := val.(*tls.Config)
		if ok == true {
			p.transport.TLSClientConfig = destTlsConfig
		} else {
			return fmt.Errorf("tls config type illegal, tls config supported")
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

			tlog.E(context.Background()).Err(err).Msgf("reset transport err (%v).",
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
	for key, val := range options {
		p.WithOption(key, val)
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

// Do issues an HTTP request with the given method and URL, merging client defaults with requestOption,
// wrapping the transport with logging and retry layers when configured, and returning a [Response] wrapper.
func (p *HttpClient) Do(ctx context.Context, method string, url string, requestOption *RequestOption, body io.Reader) (*Response, error) {
	p.RLock()
	defer p.RUnlock()

	if p.transportErr != nil {
		return nil, p.transportErr
	}

	// prepare all request configs
	// merge options
	options := make(map[int]interface{})

	for key, val := range p.options {
		options[key] = val
	}

	if requestOption != nil {
		for key, val := range requestOption.options {
			options[key] = val
		}
	}

	// merge headers
	headers := make(map[string]string)

	for key, val := range p.headers {
		headers[key] = val
	}

	if requestOption != nil {
		for key, val := range requestOption.Headers {
			headers[key] = val
		}
	}

	// set cookies
	cookies := make([]*http.Cookie, 0)

	if requestOption != nil {
		cookies = requestOption.Cookies
	}

	// transport
	transport, err := wrapTransport(p.transport, options)
	if err != nil {
		return nil, err
	}

	// cookieJar
	cookieJar, err := prepareCookieJar(options)
	if err != nil {
		return nil, err
	}

	// redirect
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
			return nil, fmt.Errorf("timeout type illegal, time.duration supported")
		} else {
			timeout = destTimeout
		}
	}

	client.Timeout = timeout

	var bodyBytes []byte

	if body != nil {
		var readErr error
		bodyBytes, readErr = io.ReadAll(body)
		if readErr != nil {
			return nil, readErr
		}

		body = bytes.NewReader(bodyBytes)
	}

	request, err := prepareRequest(ctx, method, url, headers, body)
	if err != nil {
		return nil, err
	}

	// output debug info
	if p.withDebug == true {
		dump, err := httputil.DumpRequestOut(request, true)

		if err == nil {
			tlog.I(ctx).Msgf("%s", dump)
		}
	}

	// cookieJar is not nil
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

	isAbnormal := false

	response, err := client.Do(request)
	if err != nil {
		isAbnormal = true
	} else if response != nil && response.StatusCode >= 400 {
		isAbnormal = true
	}

	if isAbnormal {
		event := tlog.W(request.Context()).Err(err).Detailf("req.method: %s", request.Method).
			Detailf("req.host: %s", request.Host).Detailf("req.url: %s", request.URL.String()).
			Detailf("req.body: %s", string(bodyBytes))

		for key, vals := range request.Header {
			event = event.Detailf("req.header.%s: %s", key, strings.Join(vals, ";"))
		}

		if response != nil {
			event = event.Detailf("resp.status code: %d", response.StatusCode)
		}

		event.Msg("http client abnormal log")
	}

	srcResponseHookFunc, ok := options[OptExtraResponseHookFunc]
	if ok == true {
		responseHookFunc, ok := srcResponseHookFunc.(ResponseHookFunc)
		if ok == true {
			responseHookFunc(response, err)
		}
	}

	return &Response{response}, err
}

// send builds a request body from params and delegates to [HttpClient.Do].
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
		return nil, fmt.Errorf("params type not support")
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
