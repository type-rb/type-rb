package stdlib

func hashSource() string {
	return `import trb/internal/hash as native_hash

def sha256(value: Bytes): Bytes
	return native_hash.sha256(value)
end

def sha512(value: Bytes): Bytes
	return native_hash.sha512(value)
end
`
}
