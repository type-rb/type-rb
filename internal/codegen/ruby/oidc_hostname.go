package ruby

const rubyOidcHostnameRuntime = `
module TrbOidcRuntime
  module_function

  def normalize_hostname(host)
    root = host.end_with?(".", "\u3002", "\uff0e", "\uff61")
    host = host[0...-1] if root
    ascii = TrbOidcIdna.to_ascii(host)
    final = ascii.split(".").last
    if final&.match?(/\A(?:[0-9]+|0x[0-9a-f]*)\z/)
      parts = ascii.split(".")
      unless parts.length == 4 && parts.all? { |part| part.match?(/\A(?:0|[1-9][0-9]{0,2})\z/) && part.to_i <= 255 }
        raise ArgumentError, "invalid OIDC hostname"
      end
    end
    root ? ascii + "." : ascii
  rescue TrbOidcIdna::IDNA::Error, EncodingError, ArgumentError
    raise ArgumentError, "invalid OIDC hostname"
  end

  def normalize_url(value)
    matched = /\A(https?:\/\/)([^\/?#]+)(.*)\z/im.match(value)
    raise ArgumentError, "OIDC endpoint must use HTTP or HTTPS" unless matched
    authority = matched[2]
    userinfo, separator, address = authority.rpartition("@")
    address = authority if separator.empty?
    if address.start_with?("[")
      # URI validates the bracketed IPv6 literal; IDNA never applies to it.
      raise ArgumentError, "invalid OIDC hostname" unless address.ascii_only?
      normalized = address
    else
      host, colon, port = address.rpartition(":")
      host = address if colon.empty?
      raise ArgumentError, "invalid OIDC hostname" if host.match?(/%[0-7][0-9a-f]/i)
      host = URI::DEFAULT_PARSER.unescape(host).force_encoding(Encoding::UTF_8)
      raise ArgumentError, "invalid OIDC hostname" unless host.valid_encoding?
      normalized = normalize_hostname(host)
      normalized += ":" + port unless colon.empty?
    end
    normalized = userinfo + "@" + normalized unless separator.empty?
    URI.parse(matched[1].downcase + normalized + matched[3])
  end

  def redirect_url(location, target)
    if location.start_with?("//")
      normalize_url(target.scheme + ":" + location)
    elsif location.match?(/\A[a-z][a-z0-9+.-]*:/i)
      normalize_url(location)
    else
      normalize_url(URI.join(target.to_s, location).to_s)
    end
  end
end
`
