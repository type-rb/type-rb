// Package pathfixture shares host composition cases across backend and REPL tests.
package pathfixture

type Fixture struct {
	Parent, Child, POSIX, Windows string
}

var Cases = []Fixture{
	{"", "a/b", "a/b", `a\b`},
	{".", "a/b", "./a/b", `.\a\b`},
	{"..", "a/b", "../a/b", `..\a\b`},
	{"/", "a/b", "/a/b", `/a\b`},
	{"//", "a/b", "//a/b", `//a\b`},
	{"///", "a/b", "///a/b", `///a\b`},
	{"/root//", "a/b", "/root//a/b", `/root//a\b`},
	{"/root/link/..", "a/b", "/root/link/../a/b", `/root/link/..\a\b`},
	{"root", "a/b", "root/a/b", `root\a\b`},
	{`root\`, "a/b", `root\/a/b`, `root\a\b`},
	{"C:", "a/b", "C:/a/b", `C:a\b`},
	{"z:", "a/b", "z:/a/b", `z:a\b`},
	{"1:", "a/b", "1:/a/b", `1:\a\b`},
	{"ab:", "a/b", "ab:/a/b", `ab:\a\b`},
	{"C:/", "a/b", "C:/a/b", `C:/a\b`},
	{`C:\`, "a/b", `C:\/a/b`, `C:\a\b`},
	{`C:link\..`, "a/b", `C:link\../a/b`, `C:link\..\a\b`},
	{`\\server\share`, "a/b", `\\server\share/a/b`, `\\server\share\a\b`},
	{`\\server\share\`, "a/b", `\\server\share\/a/b`, `\\server\share\a\b`},
	{`\\?\C:\`, "a/b", `\\?\C:\/a/b`, `\\?\C:\a\b`},
	{`\\?\UNC\server\share`, "a/b", `\\?\UNC\server\share/a/b`, `\\?\UNC\server\share\a\b`},
	{"日本語/../", "é/😀.txt", "日本語/../é/😀.txt", `日本語/../é\😀.txt`},
	{"$root/~", "file", "$root/~/file", `$root/~\file`},
}

func (f Fixture) Expected(windows bool) string {
	if windows {
		return f.Windows
	}
	return f.POSIX
}
