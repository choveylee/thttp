// Package thttp implements an HTTP client on top of net/http with composable
// transports for request logging, retries, Prometheus latency histograms, and
// optional request/response hooks.
//
// Use [NewHttpClient] to construct a client with its own [HttpClient.Transport]
// and option map, or call the package-level helpers (for example [Get], [PostJson])
// which delegate to an internal default client.
//
// Per-request settings are supplied through [RequestOption], merged with the
// client defaults when [HttpClient.Do] runs.
package thttp
