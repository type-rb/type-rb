package ruby

import (
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) authIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	if !strings.HasPrefix(name, "trb.web.auth.bearer.") && !strings.HasPrefix(name, "trb.web.auth.session.") {
		return "", false
	}
	g.oidcRuntime = true
	context := arguments[0]
	options := arguments[len(arguments)-1]
	if strings.HasPrefix(name, "trb.web.auth.session.") {
		if name == "trb.web.auth.session.start_login" {
			options = arguments[1]
		}
		return g.sessionAuthIntrinsic(name, call, arguments, context, options), true
	}
	verification := "trb_oidc_verify_bearer(" + context + ".request.headers, " + options + ".issuer, " + options + ".audience, " + options + ".jwks_uri, " + options + ".roles_claim)"
	if name == "trb.web.auth.bearer.authenticate" {
		next := arguments[1]
		return "-> { data, failure = " + verification + "; if failure; Response.new(status: 401, headers: { \"content-type\" => [\"application/json; charset=utf-8\"], \"www-authenticate\" => [\"Bearer\"] }, body: \"{\\\"error\\\":\\\"unauthorized\\\"}\".b); else; " + next + ".call(" + context + "); end }.call", true
	}
	return "-> { data, failure = " + verification + "; if failure; error = case failure[:kind]; when :missing then OidcAuthError::MissingCredentials; when :provider then OidcAuthError::Provider.new(failure[:message]); else OidcAuthError::InvalidCredentials.new(failure[:message]); end; Result::Err.new(error); else; Result::Ok.new(OidcPrincipal.new(subject: data[:subject], name: data[:name], email: data[:email], roles: data[:roles])); end }.call", true
}

func (g *generator) sessionAuthIntrinsic(name string, call *ir.Call, arguments []string, context, options string) string {
	switch name {
	case "trb.web.auth.session.start_login":
		return "trb_oidc_start_login(" + context + ".request, " + options + ", " + arguments[2] + ")"
	case "trb.web.auth.session.complete_login":
		return "trb_oidc_complete_login(" + context + ".request, " + options + ")"
	case "trb.web.auth.session.end_session":
		return "trb_oidc_end_session(" + context + ".request, " + options + ")"
	}
	verification := "trb_oidc_session_principal(" + context + ".request.headers, " + options + ".cookie_name, " + options + ".cookie_secret)"
	if name == "trb.web.auth.session.authenticate" {
		return "-> { data, session, failure = " + verification + "; if failure; trb_oidc_auth_response(401, \"unauthorized\"); elsif !trb_oidc_csrf_valid(" + context + ".request, session[:csrf]); trb_oidc_auth_response(403, \"invalid_csrf_token\"); else; " + arguments[1] + ".call(" + context + "); end }.call"
	}
	return "-> { data, _session, failure = " + verification + "; if failure; error = failure[:kind] == :missing ? OidcAuthError::MissingCredentials : OidcAuthError::InvalidCredentials.new(failure[:message]); Result::Err.new(error); else; Result::Ok.new(OidcPrincipal.new(subject: data[:subject], name: data[:name], email: data[:email], roles: data[:roles])); end }.call"
}

func (g *generator) oidcRuntimeSupport() {
	g.line(`require "base64"`, "")
	g.line(`require "json"`, "")
	g.line(`require "net/http"`, "")
	g.line(`require "openssl"`, "")
	g.line(`require "digest"`, "")
	g.line(`require "uri"`, "")
	g.line(`TRB_OIDC_JWKS_CACHE = {} unless defined?(TRB_OIDC_JWKS_CACHE)`, "")
	g.line(`TRB_OIDC_JWKS_MUTEX = Mutex.new unless defined?(TRB_OIDC_JWKS_MUTEX)`, "")
	g.line(`def trb_oidc_base64url(value)`, "")
	g.indent++
	g.line(`Base64.urlsafe_decode64(value + ("=" * ((4 - value.length % 4) % 4)))`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`def trb_oidc_verify_bearer(headers, issuer, audience, jwks_uri, roles_claim)`, "")
	g.indent++
	g.line(`values = headers.each_with_object([]) { |(name, entries), result| result.concat(entries) if name.downcase == "authorization" }`, "")
	g.line(`return [nil, { kind: :missing, message: "exactly one Authorization header is required" }] unless values.length == 1`, "")
	g.line(`parts = values.first.strip.split(/\s+/)`, "")
	g.line(`return [nil, { kind: :invalid, message: "Authorization header must use Bearer" }] unless parts.length == 2 && parts.first.downcase == "bearer"`, "")
	g.line(`trb_oidc_verify_jwt(parts.last, issuer, audience, jwks_uri, roles_claim, "")`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`def trb_oidc_verify_jwt(token, issuer, audience, jwks_uri, roles_claim, nonce)`, "")
	g.indent++
	g.line(`segments = token.split(".")`, "")
	g.line(`return [nil, { kind: :invalid, message: "JWT must contain three segments" }] unless segments.length == 3`, "")
	g.line(`header = JSON.parse(trb_oidc_base64url(segments[0]))`, "")
	g.line(`claims = JSON.parse(trb_oidc_base64url(segments[1]))`, "")
	g.line(`return [nil, { kind: :invalid, message: "JWT must use RS256 with a key id" }] unless header["alg"] == "RS256" && header["kid"].is_a?(String) && !header["kid"].empty?`, "")
	g.line(`keys, failure = trb_oidc_load_jwks(jwks_uri)`, "")
	g.line(`return [nil, failure] if failure`, "")
	g.line(`jwk = keys.find { |candidate| candidate["kid"] == header["kid"] && candidate["kty"] == "RSA" && [nil, "sig"].include?(candidate["use"]) && [nil, "RS256"].include?(candidate["alg"]) }`, "")
	g.line(`return [nil, { kind: :invalid, message: "JWT signing key was not found" }] unless jwk`, "")
	g.line(`modulus = OpenSSL::BN.new(trb_oidc_base64url(jwk.fetch("n")), 2)`, "")
	g.line(`exponent = OpenSSL::BN.new(trb_oidc_base64url(jwk.fetch("e")), 2)`, "")
	g.line(`public_key = OpenSSL::PKey::RSA.new(OpenSSL::ASN1::Sequence([OpenSSL::ASN1::Integer(modulus), OpenSSL::ASN1::Integer(exponent)]).to_der)`, "")
	g.line(`signature = trb_oidc_base64url(segments[2])`, "")
	g.line(`return [nil, { kind: :invalid, message: "JWT signature is invalid" }] unless public_key.verify(OpenSSL::Digest::SHA256.new, signature, segments[0] + "." + segments[1])`, "")
	g.line(`return [nil, { kind: :invalid, message: "JWT issuer does not match" }] unless claims["iss"] == issuer`, "")
	g.line(`audiences = claims["aud"].is_a?(Array) ? claims["aud"] : [claims["aud"]]`, "")
	g.line(`return [nil, { kind: :invalid, message: "JWT audience does not match" }] unless audiences.include?(audience)`, "")
	g.line(`return [nil, { kind: :invalid, message: "OIDC authorized party does not match" }] if !nonce.empty? && audiences.length > 1 && claims["azp"] != audience`, "")
	g.line(`now = Time.now.to_i`, "")
	g.line(`return [nil, { kind: :invalid, message: "JWT is expired" }] unless claims["exp"].is_a?(Numeric) && claims["exp"] > now - 60`, "")
	g.line(`return [nil, { kind: :invalid, message: "JWT is not active" }] if claims["nbf"].is_a?(Numeric) && claims["nbf"] > now + 60`, "")
	g.line(`return [nil, { kind: :invalid, message: "OIDC nonce does not match" }] unless nonce.empty? || claims["nonce"] == nonce`, "")
	g.line(`subject = claims["sub"]`, "")
	g.line(`return [nil, { kind: :invalid, message: "JWT subject is missing" }] unless subject.is_a?(String) && !subject.empty?`, "")
	g.line(`roles = claims[roles_claim].is_a?(Array) ? claims[roles_claim].select { |value| value.is_a?(String) } : []`, "")
	g.line(`[{ subject: subject, name: claims["name"].is_a?(String) ? claims["name"] : nil, email: claims["email"].is_a?(String) ? claims["email"] : nil, roles: roles }, nil]`, "")
	g.line(`rescue StandardError => error`, "")
	g.indent++
	g.line(`[nil, { kind: :invalid, message: error.message }]`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`def trb_oidc_load_jwks(uri_text)`, "")
	g.indent++
	g.line(`return [nil, { kind: :provider, message: "JWKS URI is empty" }] if uri_text.empty?`, "")
	g.line(`cached = TRB_OIDC_JWKS_MUTEX.synchronize { TRB_OIDC_JWKS_CACHE[uri_text] }`, "")
	g.line(`return [cached[:keys], nil] if cached && cached[:expires] > Time.now.to_i`, "")
	g.line(`uri = URI.parse(uri_text)`, "")
	g.line(`response = Net::HTTP.start(uri.host, uri.port, use_ssl: uri.scheme == "https", open_timeout: 5, read_timeout: 5) { |http| http.get(uri.request_uri) }`, "")
	g.line(`return [nil, { kind: :provider, message: "load JWKS: HTTP #{response.code}" }] unless response.code.to_i == 200`, "")
	g.line(`return [nil, { kind: :provider, message: "JWKS response is too large" }] if response.body.bytesize > 1_048_576`, "")
	g.line(`document = JSON.parse(response.body)`, "")
	g.line(`keys = document["keys"]`, "")
	g.line(`return [nil, { kind: :provider, message: "JWKS response is invalid" }] unless keys.is_a?(Array) && !keys.empty?`, "")
	g.line(`TRB_OIDC_JWKS_MUTEX.synchronize { TRB_OIDC_JWKS_CACHE[uri_text] = { keys: keys, expires: Time.now.to_i + 300 } }`, "")
	g.line(`[keys, nil]`, "")
	g.line(`rescue StandardError => error`, "")
	g.indent++
	g.line(`[nil, { kind: :provider, message: error.message }]`, "")
	g.indent--
	g.line(`end`, "")
	g.b.WriteString(rubyOidcSessionRuntime)
	g.b.WriteByte('\n')
}

const rubyOidcSessionRuntime = `
def trb_oidc_random(size)
  Base64.urlsafe_encode64(OpenSSL::Random.random_bytes(size), padding: false)
end

def trb_oidc_cookie_secret(value)
  decoded = trb_oidc_base64url(value)
  raise "OIDC cookie secret must encode exactly 32 bytes" unless decoded.bytesize == 32
  decoded
end

def trb_oidc_encrypt(value, secret_text)
  cipher = OpenSSL::Cipher.new("aes-256-gcm").encrypt
  cipher.key = trb_oidc_cookie_secret(secret_text)
  nonce = OpenSSL::Random.random_bytes(12)
  cipher.iv = nonce
  encrypted = cipher.update(JSON.generate(value)) + cipher.final
  Base64.urlsafe_encode64(nonce + cipher.auth_tag + encrypted, padding: false)
end

def trb_oidc_decrypt(value, secret_text)
  encoded = trb_oidc_base64url(value)
  raise "encrypted OIDC cookie is truncated" if encoded.bytesize < 29
  cipher = OpenSSL::Cipher.new("aes-256-gcm").decrypt
  cipher.key = trb_oidc_cookie_secret(secret_text)
  cipher.iv = encoded.byteslice(0, 12)
  cipher.auth_tag = encoded.byteslice(12, 16)
  JSON.parse(cipher.update(encoded.byteslice(28..)) + cipher.final)
end

def trb_oidc_cookies(headers)
  result = Hash.new { |hash, key| hash[key] = [] }
  headers.each do |name, entries|
    next unless name.downcase == "cookie"
    entries.each do |header|
      header.split(";").each do |part|
        cookie_name, value = part.strip.split("=", 2)
        result[cookie_name] << value if value
      end
    end
  end
  result
end

def trb_oidc_cookie(name, value, max_age, secure, http_only)
  parts = ["#{name}=#{value}", "Path=/", "Max-Age=#{max_age}", "SameSite=Lax"]
  parts << "Secure" if secure
  parts << "HttpOnly" if http_only
  parts.join("; ")
end

def trb_oidc_auth_response(status, code)
  Response.new(status: status, headers: { "content-type" => ["application/json; charset=utf-8"] }, body: JSON.generate(error: code).b)
end

def trb_oidc_start_login(_request, options, authored_return_to)
  return trb_oidc_auth_response(500, "invalid_auth_configuration") if options.authorization_endpoint.empty? || options.client_id.empty? || options.redirect_uri.empty? || options.scope.empty? || options.cookie_name.empty?
  return_to = !authored_return_to.start_with?("/") || authored_return_to.start_with?("//") || authored_return_to.match?(/[\\\r\n]/) ? "/" : authored_return_to
  state = trb_oidc_random(32)
  nonce = trb_oidc_random(32)
  verifier = trb_oidc_random(48)
  encrypted = trb_oidc_encrypt({ state: state, nonce: nonce, verifier: verifier, return_to: return_to, expires: Time.now.to_i + 600 }, options.cookie_secret)
  challenge = Base64.urlsafe_encode64(Digest::SHA256.digest(verifier), padding: false)
  uri = URI.parse(options.authorization_endpoint)
  query = URI.decode_www_form(uri.query.to_s)
  query.concat([["response_type", "code"], ["client_id", options.client_id], ["redirect_uri", options.redirect_uri], ["scope", options.scope], ["state", state], ["nonce", nonce], ["code_challenge", challenge], ["code_challenge_method", "S256"]])
  query << ["audience", options.audience] if options.audience
  uri.query = URI.encode_www_form(query)
  Response.new(status: 302, headers: { "location" => [uri.to_s], "set-cookie" => [trb_oidc_cookie(options.cookie_name + "_state", encrypted, 600, options.secure, true)] }, body: "".b)
rescue StandardError
  trb_oidc_auth_response(500, "invalid_auth_configuration")
end

def trb_oidc_query_value(raw, name)
  values = URI.decode_www_form(raw).select { |key, _value| key == name }.map(&:last)
  values.length == 1 && !values.first.empty? ? values.first : nil
rescue StandardError
  nil
end

def trb_oidc_complete_login(request, options)
  code = trb_oidc_query_value(request.query_string, "code")
  state = trb_oidc_query_value(request.query_string, "state")
  state_cookies = trb_oidc_cookies(request.headers)[options.cookie_name + "_state"]
  return trb_oidc_auth_response(400, "invalid_oidc_callback") unless code && state && state_cookies.length == 1
  login = trb_oidc_decrypt(state_cookies.first, options.cookie_secret)
  return trb_oidc_auth_response(400, "invalid_oidc_state") if login.fetch("expires") < Time.now.to_i || !trb_oidc_constant_time(login.fetch("state"), state)
  uri = URI.parse(options.token_endpoint)
  form = URI.encode_www_form(grant_type: "authorization_code", code: code, redirect_uri: options.redirect_uri, code_verifier: login.fetch("verifier"))
  token_request = Net::HTTP::Post.new(uri.request_uri)
  token_request["content-type"] = "application/x-www-form-urlencoded"
  token_request.basic_auth(options.client_id, options.client_secret)
  token_request.body = form
  token_response = Net::HTTP.start(uri.host, uri.port, use_ssl: uri.scheme == "https", open_timeout: 5, read_timeout: 10) { |http| http.request(token_request) }
  return trb_oidc_auth_response(502, "identity_provider_error") unless token_response.code.to_i == 200 && token_response.body.bytesize <= 1_048_576
  tokens = JSON.parse(token_response.body)
  return trb_oidc_auth_response(502, "identity_provider_error") unless tokens["id_token"].is_a?(String)
  principal, failure = trb_oidc_verify_jwt(tokens["id_token"], options.issuer, options.client_id, options.jwks_uri, options.roles_claim, login.fetch("nonce"))
  return trb_oidc_auth_response(400, "invalid_identity_token") if failure
  csrf = trb_oidc_random(32)
  session = { subject: principal[:subject], name: principal[:name].to_s, email: principal[:email].to_s, roles: principal[:roles], csrf: csrf, id_token: tokens["id_token"], expires: Time.now.to_i + 28_800 }
  encrypted = trb_oidc_encrypt(session, options.cookie_secret)
  Response.new(status: 302, headers: { "location" => [login.fetch("return_to")], "set-cookie" => [trb_oidc_cookie(options.cookie_name, encrypted, 28_800, options.secure, true), trb_oidc_cookie("trb_csrf", URI.encode_www_form_component(csrf), 28_800, options.secure, false), trb_oidc_cookie(options.cookie_name + "_state", "", 0, options.secure, true)] }, body: "".b)
rescue StandardError
  trb_oidc_auth_response(502, "identity_provider_unavailable")
end

def trb_oidc_session_principal(headers, cookie_name, secret)
  values = trb_oidc_cookies(headers)[cookie_name]
  return [nil, nil, { kind: :missing, message: "session cookie is missing" }] unless values.length == 1
  session = trb_oidc_decrypt(values.first, secret)
  return [nil, nil, { kind: :invalid, message: "session cookie is invalid" }] if session.fetch("expires") <= Time.now.to_i || session.fetch("subject").empty?
  [{ subject: session.fetch("subject"), name: session.fetch("name").empty? ? nil : session.fetch("name"), email: session.fetch("email").empty? ? nil : session.fetch("email"), roles: session.fetch("roles") }, session.transform_keys(&:to_sym), nil]
rescue StandardError
  [nil, nil, { kind: :invalid, message: "session cookie is invalid" }]
end

def trb_oidc_constant_time(left, right)
  maximum = [left.bytesize, right.bytesize].max
  difference = left.bytesize ^ right.bytesize
  maximum.times { |index| difference |= (left.getbyte(index) || 0) ^ (right.getbyte(index) || 0) }
  difference.zero?
end

def trb_oidc_csrf_valid(request, expected)
  return true if ["GET", "HEAD", "OPTIONS"].include?(request.method.upcase)
  cookie_values = trb_oidc_cookies(request.headers)["trb_csrf"]
  header_values = request.headers.each_with_object([]) { |(name, values), result| result.concat(values) if name.downcase == "x-csrf-token" }
  return false unless cookie_values.length == 1 && header_values.length == 1
  cookie_value = URI.decode_www_form_component(cookie_values.first)
  trb_oidc_constant_time(expected, cookie_value) && trb_oidc_constant_time(expected, header_values.first)
rescue StandardError
  false
end

def trb_oidc_end_session(request, options)
  location = options.post_logout_redirect_uri
  if options.end_session_endpoint
    uri = URI.parse(options.end_session_endpoint)
    query = URI.decode_www_form(uri.query.to_s)
    query.concat([["post_logout_redirect_uri", options.post_logout_redirect_uri], ["client_id", options.client_id]])
    _principal, session, failure = trb_oidc_session_principal(request.headers, options.cookie_name, options.cookie_secret)
    query << ["id_token_hint", session[:id_token]] if !failure && !session[:id_token].empty?
    uri.query = URI.encode_www_form(query)
    location = uri.to_s
  end
  Response.new(status: 302, headers: { "location" => [location], "set-cookie" => [trb_oidc_cookie(options.cookie_name, "", 0, options.secure, true), trb_oidc_cookie("trb_csrf", "", 0, options.secure, false)] }, body: "".b)
end
`
