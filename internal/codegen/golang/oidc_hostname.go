package golang

const goOidcHostnameRuntime = `
var trbOidcIDNA = idna.New(idna.MapForLookup(), idna.Transitional(false),
 idna.StrictDomainName(true), idna.ValidateLabels(true), idna.CheckHyphens(true),
 idna.CheckJoiners(true), idna.BidiRule(), idna.VerifyDNSLength(true))

func trbOidcHostname(host string) (string, error) {
 if address, err := netip.ParseAddr(host); err == nil { return address.String(), nil }
 root := false
 for _, dot := range []string{".", "\u3002", "\uff0e", "\uff61"} {
  if strings.HasSuffix(host, dot) { host = strings.TrimSuffix(host, dot); root = true; break }
 }
 ascii, err := trbOidcIDNA.ToASCII(host)
 if err != nil { return "", fmt.Errorf("invalid OIDC hostname") }
 // WHATWG URL parsers interpret numeric final labels as IPv4. Only accept
 // canonical dotted decimal here so every output connects to the same host.
 final := ascii[strings.LastIndex(ascii, ".") + 1:]
 numeric := final != ""
 for _, c := range final { if c < '0' || c > '9' { numeric = false } }
 if strings.HasPrefix(final, "0x") {
  numeric = true
  for _, c := range final[2:] { if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') { numeric = false } }
 }
 if numeric {
  address, err := netip.ParseAddr(ascii)
  if err != nil || !address.Is4() || address.String() != ascii { return "", fmt.Errorf("invalid OIDC hostname") }
 }
 if root { ascii += "." }
 return ascii, nil
}

func trbOidcNormalizeURL(target *url.URL) (*url.URL, error) {
 if target.Scheme != "http" && target.Scheme != "https" { return nil, fmt.Errorf("OIDC endpoint must use HTTP or HTTPS") }
 if strings.HasPrefix(target.Host, "[") {
  address, err := netip.ParseAddr(target.Hostname())
  if err != nil || !address.Is6() || address.Zone() != "" { return nil, fmt.Errorf("invalid OIDC hostname") }
 }
 host, err := trbOidcHostname(target.Hostname())
 if err != nil { return nil, err }
 copy := *target
 if strings.Contains(host, ":") { host = "[" + host + "]" }
 if port := target.Port(); port != "" { host += ":" + port }
 copy.Host = host
 return &copy, nil
}

type trbOidcTransport struct{}
func (trbOidcTransport) RoundTrip(request *nethttp.Request) (*nethttp.Response, error) {
 target, err := trbOidcNormalizeURL(request.URL)
 if err != nil { return nil, err }
 copy := request.Clone(request.Context())
 copy.URL = target
 copy.Host = target.Host
 return nethttp.DefaultTransport.RoundTrip(copy)
}
`
