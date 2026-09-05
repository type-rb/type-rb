//go:build !linux && !darwin

package repl

func regularFileOpenFlags() (int, bool) {
	return 0, false
}
