package golang

import (
	pathpkg "path"
	"strings"
)

// GeneratedSourceDirectory maps source-only route parameter segments to Go
// package path segments while leaving ordinary TypeRB module directories
// unchanged.
func GeneratedSourceDirectory(directory string) string {
	if directory == "" || directory == "." {
		return directory
	}
	segments := strings.Split(pathpkg.Clean(directory), "/")
	for index, segment := range segments {
		segments[index] = generatedSourceDirectorySegment(segment)
	}
	return strings.Join(segments, "/")
}

func generatedSourceDirectorySegment(segment string) string {
	if len(segment) < 3 || segment[0] != '[' || segment[len(segment)-1] != ']' {
		return segment
	}
	parameter := segment[1 : len(segment)-1]
	prefix := "route_param_"
	if strings.HasPrefix(parameter, "...") {
		parameter = strings.TrimPrefix(parameter, "...")
		prefix = "route_catch_all_"
	}
	if parameter == "" {
		return segment
	}
	for index, character := range parameter {
		if character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return segment
	}
	return prefix + parameter
}
