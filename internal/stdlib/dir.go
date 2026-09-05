package stdlib

func dirSource() string {
	return `class Dir
end

enum DirEntryKind
	File
	Directory
	Other
end

record DirEntry<P>
	name: String
	path: P
	kind: DirEntryKind
end
`
}
