package transport

import "net/http"

// HeaderTransport returns an *http.Client that injects custom headers into
// every request. If base is nil, http.DefaultTransport is used.
func HeaderTransport(base http.RoundTripper, headers map[string]string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &headerRoundTripper{base: base, headers: headers}
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone to avoid mutating the original request.
	clone := req.Clone(req.Context())
	for k, v := range h.headers {
		clone.Header.Set(k, v)
	}
	return h.base.RoundTrip(clone)
}
