package stdlib

func hmacSource() string {
	return `import trb/internal/hmac as native_hmac

def sha256(key: Bytes, message: Bytes): Bytes
	return native_hmac.sha256(key, message)
end

def sha512(key: Bytes, message: Bytes): Bytes
	return native_hmac.sha512(key, message)
end

def equal(left: Bytes, right: Bytes): Boolean
	return native_hmac.equal(left, right)
end
`
}
