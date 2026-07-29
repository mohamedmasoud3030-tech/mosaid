package research

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type Hop struct {
	URL       *url.URL
	Addresses []netip.Addr
}

type TransportFactory interface {
	TransportFor(Hop) (http.RoundTripper, error)
}

type FetchPolicy struct {
	AllowedHosts      []string
	AllowedPorts      []int
	AllowedTypes      []string
	MaxBytes          int64
	MaxRedirects      int
	Timeout           time.Duration
	MaxResponseHeader int64
	UserAgent         string
}

type Fetcher struct {
	Policy     FetchPolicy
	Resolver   Resolver
	Transports TransportFactory
	Clock      Clock
}

func NewFetcher(policy FetchPolicy) (*Fetcher, error) {
	policy = normalizedPolicy(policy)
	if err := validatePolicy(policy); err != nil {
		return nil, err
	}
	return &Fetcher{Policy: policy, Resolver: net.DefaultResolver, Transports: pinnedTransportFactory{policy: policy}, Clock: RealClock{}}, nil
}

func normalizedPolicy(policy FetchPolicy) FetchPolicy {
	if len(policy.AllowedPorts) == 0 {
		policy.AllowedPorts = []int{80, 443}
	}
	if len(policy.AllowedTypes) == 0 {
		policy.AllowedTypes = []string{"text/plain", "text/html", "text/markdown", "application/json", "application/xml", "text/xml"}
	}
	if policy.MaxBytes == 0 {
		policy.MaxBytes = 2 * 1024 * 1024
	}
	if policy.MaxRedirects == 0 {
		policy.MaxRedirects = 4
	}
	if policy.Timeout == 0 {
		policy.Timeout = 20 * time.Second
	}
	if policy.MaxResponseHeader == 0 {
		policy.MaxResponseHeader = 64 * 1024
	}
	if policy.UserAgent == "" {
		policy.UserAgent = "Mosaid/0.1 research-fetcher"
	}
	return policy
}

func validatePolicy(policy FetchPolicy) error {
	if policy.MaxBytes < 1 || policy.MaxBytes > 8*1024*1024 || policy.MaxRedirects < 0 || policy.MaxRedirects > 8 || policy.Timeout <= 0 || policy.Timeout > time.Minute || policy.MaxResponseHeader < 1024 || policy.MaxResponseHeader > 1024*1024 {
		return errors.New("invalid fetch limits")
	}
	for _, port := range policy.AllowedPorts {
		if port < 1 || port > 65535 {
			return errors.New("invalid allowed port")
		}
	}
	for _, host := range policy.AllowedHosts {
		if host == "" || host != strings.ToLower(host) || strings.ContainsAny(host, "/*:@") {
			return errors.New("allowed hosts must be exact lowercase DNS names")
		}
	}
	for _, contentType := range policy.AllowedTypes {
		if contentType != strings.ToLower(contentType) || strings.ContainsAny(contentType, " ;") {
			return errors.New("invalid allowed content type")
		}
	}
	return nil
}

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (ExternalContent, error) {
	if f == nil || f.Resolver == nil || f.Transports == nil || f.Clock == nil {
		return ExternalContent{}, errors.New("fetcher dependencies unavailable")
	}
	policy := normalizedPolicy(f.Policy)
	if err := validatePolicy(policy); err != nil {
		return ExternalContent{}, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
	defer cancel()
	current, err := parseExternalURL(rawURL, policy)
	if err != nil {
		return ExternalContent{}, err
	}
	for redirect := 0; ; redirect++ {
		hop, err := f.resolveHop(fetchCtx, current, policy)
		if err != nil {
			if redirect > 0 {
				return ExternalContent{}, fmt.Errorf("%w: %w", ErrRedirectDenied, err)
			}
			return ExternalContent{}, err
		}
		transport, err := f.Transports.TransportFor(hop)
		if err != nil {
			return ExternalContent{}, err
		}
		client := &http.Client{
			Transport: transport,
			Timeout:   policy.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		request, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, current.String(), nil)
		if err != nil {
			return ExternalContent{}, err
		}
		request.Header.Set("Accept", strings.Join(policy.AllowedTypes, ", "))
		request.Header.Set("User-Agent", policy.UserAgent)
		response, err := client.Do(request)
		if err != nil {
			return ExternalContent{}, err
		}
		if isRedirect(response.StatusCode) {
			location := response.Header.Get("Location")
			response.Body.Close()
			if redirect >= policy.MaxRedirects || location == "" {
				return ExternalContent{}, ErrRedirectDenied
			}
			next, err := current.Parse(location)
			if err != nil {
				return ExternalContent{}, fmt.Errorf("%w: malformed location", ErrRedirectDenied)
			}
			current, err = parseExternalURL(next.String(), policy)
			if err != nil {
				return ExternalContent{}, fmt.Errorf("%w: %v", ErrRedirectDenied, err)
			}
			continue
		}
		content, err := f.readResponse(response, current, policy)
		response.Body.Close()
		return content, err
	}
}

func (f *Fetcher) resolveHop(ctx context.Context, target *url.URL, policy FetchPolicy) (Hop, error) {
	host := strings.ToLower(target.Hostname())
	var addresses []netip.Addr
	if literal, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{literal.Unmap()}
	} else {
		resolved, err := f.Resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return Hop{}, err
		}
		for _, item := range resolved {
			address, ok := netip.AddrFromSlice(item.IP)
			if !ok {
				return Hop{}, ErrAddressDenied
			}
			addresses = append(addresses, address.Unmap())
		}
	}
	if len(addresses) == 0 {
		return Hop{}, ErrAddressDenied
	}
	seen := map[netip.Addr]struct{}{}
	unique := addresses[:0]
	for _, address := range addresses {
		if deniedAddress(address) {
			return Hop{}, fmt.Errorf("%w: non-public destination", ErrAddressDenied)
		}
		if _, exists := seen[address]; !exists {
			seen[address] = struct{}{}
			unique = append(unique, address)
		}
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i].Compare(unique[j]) < 0 })
	return Hop{URL: cloneURL(target), Addresses: unique}, nil
}

func parseExternalURL(raw string, policy FetchPolicy) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > 8192 {
		return nil, ErrURLDenied
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, ErrURLDenied
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host != parsed.Hostname() || isMetadataHostname(host) || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, ErrURLDenied
	}
	if literal, err := netip.ParseAddr(host); err == nil && deniedAddress(literal.Unmap()) {
		return nil, ErrURLDenied
	}
	for _, character := range host {
		if character > 127 {
			return nil, ErrURLDenied
		}
	}
	if len(policy.AllowedHosts) != 0 && !stringContains(policy.AllowedHosts, host) {
		return nil, ErrURLDenied
	}
	port := 80
	if parsed.Scheme == "https" {
		port = 443
	}
	explicitPort := parsed.Port() != ""
	if explicitPort {
		value, err := strconv.Atoi(parsed.Port())
		if err != nil {
			return nil, ErrURLDenied
		}
		port = value
	}
	if !intContains(policy.AllowedPorts, port) {
		return nil, ErrURLDenied
	}
	parsed.Host = host
	if explicitPort {
		parsed.Host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return parsed, nil
}

func (f *Fetcher) readResponse(response *http.Response, finalURL *url.URL, policy FetchPolicy) (ExternalContent, error) {
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return ExternalContent{}, fmt.Errorf("external HTTP status %d", response.StatusCode)
	}
	if encoding := strings.TrimSpace(response.Header.Get("Content-Encoding")); encoding != "" && encoding != "identity" {
		return ExternalContent{}, fmt.Errorf("%w: encoded bodies are disabled", ErrContentType)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !stringContains(policy.AllowedTypes, strings.ToLower(mediaType)) {
		return ExternalContent{}, ErrContentType
	}
	if response.ContentLength > policy.MaxBytes {
		return ExternalContent{}, ErrContentTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, policy.MaxBytes+1))
	if err != nil {
		return ExternalContent{}, err
	}
	if int64(len(body)) > policy.MaxBytes {
		return ExternalContent{}, ErrContentTooLarge
	}
	if !utf8.Valid(body) || (mediaType == "application/json" && !json.Valid(body)) {
		return ExternalContent{}, ErrContentType
	}
	provenance := Provenance{URL: finalURL.String(), RetrievedAt: f.Clock.Now().UTC(), SHA256: HashBytes(body), ContentType: strings.ToLower(mediaType), Bytes: len(body)}
	return NewExternalContent(string(body), provenance), nil
}

func deniedAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return true
	}
	blocked := []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
	for _, prefix := range blocked {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func isMetadataHostname(host string) bool {
	switch host {
	case "metadata.google.internal", "metadata.goog", "instance-data", "instance-data.ec2.internal", "metadata.azure.internal":
		return true
	default:
		return false
	}
}

func isRedirect(status int) bool {
	return status == http.StatusMovedPermanently || status == http.StatusFound || status == http.StatusSeeOther || status == http.StatusTemporaryRedirect || status == http.StatusPermanentRedirect
}

type pinnedTransportFactory struct{ policy FetchPolicy }

func (p pinnedTransportFactory) TransportFor(hop Hop) (http.RoundTripper, error) {
	if len(hop.Addresses) == 0 {
		return nil, ErrAddressDenied
	}
	port := hop.URL.Port()
	if port == "" {
		if hop.URL.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	destination := net.JoinHostPort(hop.Addresses[0].String(), port)
	dialer := &net.Dialer{Timeout: p.policy.Timeout / 2, KeepAlive: -1}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, destination)
		},
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, ServerName: hop.URL.Hostname()},
		TLSHandshakeTimeout:    p.policy.Timeout / 2,
		ResponseHeaderTimeout:  p.policy.Timeout / 2,
		MaxResponseHeaderBytes: p.policy.MaxResponseHeader,
		DisableCompression:     true,
		DisableKeepAlives:      true,
	}, nil
}

func cloneURL(input *url.URL) *url.URL {
	copy := *input
	return &copy
}

func stringContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func intContains(values []int, wanted int) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
