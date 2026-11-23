package stdlib

func fileSource() string {
	return `class File
end

enum FileMode
	Read
	Write
	CreateNew
end
`
}
