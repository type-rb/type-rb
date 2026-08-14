package ruby

import "github.com/type-rb/type-rb/internal/ir"

const oidcVerifyBearerIntrinsic = "trb.internal.auth.oidc.verify_bearer"

func (g *generator) oidcIntrinsic(name string, _ *ir.Call, arguments []string) (string, bool) {
	if name != oidcVerifyBearerIntrinsic {
		return "", false
	}
	g.oidcRuntime = true
	if len(arguments) != 2 {
		return "nil", true
	}
	return "->(request_value, options_value) { authorization = request_value.__trb_field_headers.entries.select { |header| header.name.casecmp?(\"authorization\") }.map(&:value); verified = TrbOidcRuntime.verify_bearer(authorization, options_value.issuer, options_value.audience, options_value.jwks_uri, options_value.roles_claim, options_value.clock_skew_seconds); if verified[:failure]; failure = verified[:failure]; kind = case failure[:kind]; when :missing then OidcAuthErrorKind::MissingCredentials; when :provider then OidcAuthErrorKind::Provider; when :configuration then OidcAuthErrorKind::Configuration; else OidcAuthErrorKind::InvalidCredentials; end; Result::Err.new(OidcAuthError.new(kind: kind, message: failure[:message])); else; data = verified[:data]; Result::Ok.new(OidcPrincipal.new(subject: data[:subject], name: data[:name], email: data[:email], roles: data[:roles])); end }.call(" + arguments[0] + ", " + arguments[1] + ")", true
}

func (g *generator) oidcBearerRuntimeSupport() {
	for _, library := range []string{"base64", "json", "net/http", "openssl", "uri"} {
		g.line("require "+quoteRuby(library), "")
	}
	g.b.WriteString(rubyOidcBearerRuntime)
	g.b.WriteByte('\n')
}

func quoteRuby(value string) string {
	return `"` + value + `"`
}

const rubyOidcBearerRuntime = `
module TrbOidcRuntime
  HTTP_LIMIT = 1_048_576
  PROVIDER_CACHE = {}
  JWKS_CACHE = {}
  CACHE_LOCK = Mutex.new

  module_function

  def get_json(uri, label)
    target = URI.parse(uri)
    client = Net::HTTP.new(target.host, target.port)
    client.use_ssl = target.scheme == "https"
    client.open_timeout = 5
    client.read_timeout = 5
    response = client.request(Net::HTTP::Get.new(target.request_uri))
    return { failure: { kind: :provider, message: "#{label}: HTTP #{response.code}" } } unless response.is_a?(Net::HTTPSuccess)
    body = response.body.to_s
    return { failure: { kind: :provider, message: "#{label} response is too large" } } if body.bytesize > HTTP_LIMIT
    { value: JSON.parse(body) }
  rescue StandardError => error
    { failure: { kind: :provider, message: "#{label}: #{error.message}" } }
  end

  def load_provider(issuer)
    return { failure: { kind: :configuration, message: "OIDC issuer is empty" } } if issuer.empty?
    now = Process.clock_gettime(Process::CLOCK_REALTIME)
    cached = CACHE_LOCK.synchronize { PROVIDER_CACHE[issuer] }
    return { value: cached[:value] } if cached && cached[:expires] > now
    loaded = get_json(issuer.sub(%r{/+\z}, "") + "/.well-known/openid-configuration", "load OIDC discovery")
    return loaded if loaded[:failure]
    metadata = loaded[:value]
    unless metadata.is_a?(Hash) && metadata["issuer"] == issuer && metadata["jwks_uri"].is_a?(String) && !metadata["jwks_uri"].empty?
      return { failure: { kind: :provider, message: "OIDC discovery response is invalid" } }
    end
    CACHE_LOCK.synchronize { PROVIDER_CACHE[issuer] = { value: metadata, expires: now + 300 } }
    { value: metadata }
  end

  def load_jwks(uri, force)
    return { failure: { kind: :configuration, message: "OIDC JWKS URI is empty" } } if uri.empty?
    now = Process.clock_gettime(Process::CLOCK_REALTIME)
    rate_limited = false
    cached = CACHE_LOCK.synchronize do
      value = JWKS_CACHE[uri]
      if force && value
        if value.fetch(:refresh_after, 0) > now
          rate_limited = true
        else
          value = value.merge(refresh_after: now + 30)
          JWKS_CACHE[uri] = value
        end
      end
      value
    end
    return { value: cached[:keys] } if rate_limited
    return { value: cached[:keys] } if !force && cached && cached[:expires] > now
    loaded = get_json(uri, "load OIDC JWKS")
    return loaded if loaded[:failure]
    document = loaded[:value]
    keys = document.is_a?(Hash) ? document["keys"] : nil
    return { failure: { kind: :provider, message: "OIDC JWKS response is invalid" } } unless keys.is_a?(Array) && !keys.empty?
    refresh_after = force ? now + 30 : cached&.fetch(:refresh_after, 0).to_f
    CACHE_LOCK.synchronize { JWKS_CACHE[uri] = { keys: keys, expires: now + 300, refresh_after: refresh_after } }
    { value: keys }
  end

  def select_jwk(keys, kid)
    keys.find do |key|
      key.is_a?(Hash) && key["kid"] == kid && key["kty"] == "RSA" &&
        [nil, "", "sig"].include?(key["use"]) && [nil, "", "RS256"].include?(key["alg"])
    end
  end

  def base64url(value)
    Base64.urlsafe_decode64(value + "=" * ((4 - value.length % 4) % 4))
  end

  def verify_bearer(values, issuer, audience, jwks_uri, roles_claim, clock_skew_seconds)
    if issuer.empty? || audience.empty? || roles_claim.empty? || clock_skew_seconds.negative?
      return { failure: { kind: :configuration, message: "OIDC bearer configuration is invalid" } }
    end
    return { failure: { kind: :missing, message: "Authorization header is required" } } if values.empty?
    return { failure: { kind: :invalid, message: "exactly one Authorization header is required" } } unless values.length == 1
    parts = values.first.to_s.split
    unless parts.length == 2 && parts.first.casecmp?("Bearer")
      return { failure: { kind: :invalid, message: "Authorization header must use Bearer" } }
    end
    verify_jwt(parts.last, issuer, audience, jwks_uri, roles_claim, clock_skew_seconds)
  end

  def verify_jwt(token, issuer, audience, jwks_uri, roles_claim, clock_skew_seconds)
    if jwks_uri.nil? || jwks_uri.empty?
      provider = load_provider(issuer)
      return provider if provider[:failure]
      jwks_uri = provider[:value]["jwks_uri"]
    end
    segments = token.split(".", -1)
    return invalid("JWT must contain three segments") unless segments.length == 3
    header = JSON.parse(base64url(segments[0]))
    claims = JSON.parse(base64url(segments[1]))
    signature = base64url(segments[2])
    return invalid("JWT must use RS256 with a key id") unless header.is_a?(Hash) && header["alg"] == "RS256" && header["kid"].is_a?(String) && !header["kid"].empty?
    loaded = load_jwks(jwks_uri, false)
    return loaded if loaded[:failure]
    key = select_jwk(loaded[:value], header["kid"])
    unless key
      loaded = load_jwks(jwks_uri, true)
      return loaded if loaded[:failure]
      key = select_jwk(loaded[:value], header["kid"])
    end
    return invalid("JWT signing key was not found") unless key
    modulus = OpenSSL::BN.new(base64url(key["n"]), 2)
    exponent = OpenSSL::BN.new(base64url(key["e"]), 2)
    return invalid("OIDC signing key is invalid") if modulus.num_bits < 2048 || exponent.to_i < 3 || exponent.to_i.even?
    rsa = OpenSSL::PKey::RSA.new(OpenSSL::ASN1::Sequence([OpenSSL::ASN1::Integer(modulus), OpenSSL::ASN1::Integer(exponent)]).to_der)
    return invalid("JWT signature is invalid") unless rsa.verify(OpenSSL::Digest::SHA256.new, signature, segments[0] + "." + segments[1])
    return invalid("JWT claims are invalid") unless claims.is_a?(Hash)
    return invalid("JWT issuer does not match") unless claims["iss"] == issuer
    audiences = claims["aud"].is_a?(String) ? [claims["aud"]] : claims["aud"]
    return invalid("JWT audience does not match") unless audiences.is_a?(Array) && audiences.include?(audience)
    now = Time.now.to_i
    expiration = claims["exp"]
    return invalid("JWT is expired") unless expiration.is_a?(Numeric) && expiration.to_i > now - clock_skew_seconds
    not_before = claims["nbf"]
    return invalid("JWT is not active") if not_before.is_a?(Numeric) && not_before.to_i > now + clock_skew_seconds
    subject = claims["sub"]
    return invalid("JWT subject is missing") unless subject.is_a?(String) && !subject.empty?
    roles = claims.fetch(roles_claim, [])
    return invalid("JWT roles claim is invalid") unless roles.is_a?(Array) && roles.all? { |role| role.is_a?(String) }
    name = claims["name"].is_a?(String) ? claims["name"] : nil
    email = claims["email"].is_a?(String) ? claims["email"] : nil
    { data: { subject: subject, name: name, email: email, roles: roles } }
  rescue JSON::ParserError, ArgumentError, OpenSSL::OpenSSLError
    invalid("JWT is invalid")
  rescue StandardError => error
    { failure: { kind: :invalid, message: error.message } }
  end

  def invalid(message)
    { failure: { kind: :invalid, message: message } }
  end
end
`
