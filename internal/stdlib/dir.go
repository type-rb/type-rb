package stdlib

func dirSource() string {
	return `class Dir
end

enum DirEntryKind
	File
	Directory
	Other
end

record DirEntry
	name: String
	path: String
	kind: DirEntryKind
end
`
}
