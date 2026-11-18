package thttp

import (
	"fmt"
	"net/http"
	"net/url"
)

// requestCookieJar holds explicit cookies for one Do call and delegates
// server-set cookie storage to the configured jar.
type requestCookieJar struct {
	shared http.CookieJar
	scoped http.CookieJar
}

// newRequestCookieJar seeds explicit cookies at the outbound request's origin.
// net/http uses the Host override when querying a cookie jar.
func newRequestCookieJar(shared http.CookieJar, request *http.Request, cookies []*http.Cookie) (*requestCookieJar, error) {
	scoped, err := newCookieJar()
	if err != nil {
		return nil, fmt.Errorf("thttp: create request cookie jar: %w", err)
	}

	validCookies := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie != nil {
			validCookies = append(validCookies, cookie)
		}
	}

	origin := *request.URL
	if request.Host != "" {
		origin.Host = request.Host
	}

	scoped.SetCookies(&origin, validCookies)
	jar := &requestCookieJar{shared: shared, scoped: scoped}

	return jar, nil
}

// Cookies gives matching explicit cookies precedence over stored cookies of
// the same name, while both jars apply their own scope checks.
func (jar *requestCookieJar) Cookies(target *url.URL) []*http.Cookie {
	cookies := jar.scoped.Cookies(target)
	if jar.shared == nil {
		return cookies
	}

	storedCookies := jar.shared.Cookies(target)
	if len(cookies) == 0 {
		return storedCookies
	}

	explicitNamesMap := make(map[string]struct{}, len(cookies))
	for _, cookie := range cookies {
		explicitNamesMap[cookie.Name] = struct{}{}
	}

	for _, cookie := range storedCookies {
		_, ok := explicitNamesMap[cookie.Name]
		if !ok {
			cookies = append(cookies, cookie)
		}
	}

	return cookies
}

// SetCookies stores server cookies and drops explicit entries replaced or
// deleted by the server. The scoped jar handles the domain and path match.
func (jar *requestCookieJar) SetCookies(target *url.URL, cookies []*http.Cookie) {
	if jar.shared != nil {
		jar.shared.SetCookies(target, cookies)
	}

	for _, cookie := range cookies {
		removed := *cookie
		removed.MaxAge = -1
		jar.scoped.SetCookies(target, []*http.Cookie{&removed})
	}
}
