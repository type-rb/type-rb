// Package sourcemap records backend-independent relationships between
// generated code and TypeRB source. Target-specific source-map encodings are
// deliberately layered on top of this model.
package sourcemap

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/token"
)

const Version = 1

type Location struct {
	Path string
	Span token.Span
}

type Mapping struct {
	Generated token.Span
	Source    Location
}

type Map struct {
	Version  int
	Mappings []Mapping
}

type Generated struct {
	Output string
	Map    Map
}

func (m Map) SourceAt(generated token.Position) (Location, bool) {
	best := -1
	bestWidth := 0
	for index, mapping := range m.Mappings {
		if !contains(mapping.Generated, generated) {
			continue
		}
		width := mapping.Generated.End.Offset - mapping.Generated.Start.Offset
		if best == -1 || width < bestWidth {
			best = index
			bestWidth = width
		}
	}
	if best == -1 {
		return Location{}, false
	}
	return m.Mappings[best].Source, true
}

func contains(span token.Span, position token.Position) bool {
	return comparePosition(span.Start, position) <= 0 && comparePosition(position, span.End) < 0
}

func comparePosition(left, right token.Position) int {
	if left.Line != right.Line {
		return left.Line - right.Line
	}
	return left.Column - right.Column
}

type offsetMapping struct {
	start  int
	end    int
	source Location
}

type Recorder struct {
	path     string
	mappings []offsetMapping
}

func NewRecorder(path string) *Recorder {
	return &Recorder{path: path}
}

func (r *Recorder) Record(start, end int, source token.Span) {
	if r == nil || end <= start || source.Start.Line <= 0 {
		return
	}
	r.mappings = append(r.mappings, offsetMapping{start: start, end: end, source: Location{Path: r.path, Span: source}})
}

func (r *Recorder) Build(output string) Map {
	if r == nil || len(r.mappings) == 0 {
		return Map{Version: Version}
	}
	type normalizedMapping struct {
		start  int
		end    int
		source Location
	}
	normalized := make([]normalizedMapping, 0, len(r.mappings))
	offsets := make([]int, 0, len(r.mappings)*2)
	seenOffsets := map[int]bool{}
	addOffset := func(offset int) {
		if !seenOffsets[offset] {
			seenOffsets[offset] = true
			offsets = append(offsets, offset)
		}
	}
	for _, mapping := range r.mappings {
		start := min(max(mapping.start, 0), len(output))
		end := min(max(mapping.end, 0), len(output))
		if end <= start {
			continue
		}
		normalized = append(normalized, normalizedMapping{start: start, end: end, source: mapping.source})
		addOffset(start)
		addOffset(end)
	}
	positions := positionsAt(output, offsets)
	result := Map{Version: Version}
	for _, mapping := range normalized {
		result.Mappings = append(result.Mappings, Mapping{
			Generated: token.Span{Start: positions[mapping.start], End: positions[mapping.end]},
			Source:    mapping.source,
		})
	}
	return result
}

func PositionAt(source string, offset int) token.Position {
	offset = min(max(offset, 0), len(source))
	return positionsAt(source, []int{offset})[offset]
}

func positionsAt(source string, offsets []int) map[int]token.Position {
	offsets = append([]int(nil), offsets...)
	sort.Ints(offsets)
	result := make(map[int]token.Position, len(offsets))
	position := token.Position{Offset: 0, Line: 1, Column: 1}
	for _, offset := range offsets {
		offset = min(max(offset, 0), len(source))
		for position.Offset < offset {
			if source[position.Offset] == '\n' {
				position.Offset++
				position.Line++
				position.Column = 1
				continue
			}
			_, width := utf8.DecodeRuneInString(source[position.Offset:])
			if width < 1 {
				width = 1
			}
			if position.Offset+width > offset {
				width = offset - position.Offset
			}
			position.Offset += width
			position.Column++
		}
		result[offset] = position
	}
	return result
}

const markerPrefix = "__trb_source_"

func StartMarker(id int) string {
	return "// " + markerPrefix + "start_" + strconv.Itoa(id)
}

func EndMarker(id int) string {
	return "// " + markerPrefix + "end_" + strconv.Itoa(id)
}

// ExtractMarkers removes codegen-only marker lines after target formatting and
// converts their byte ranges into the shared mapping model.
func ExtractMarkers(marked string, locations map[int]Location) (string, Map) {
	starts := map[int]int{}
	recorder := &Recorder{}
	var output strings.Builder
	for len(marked) > 0 {
		line := marked
		if newline := strings.IndexByte(marked, '\n'); newline >= 0 {
			line = marked[:newline+1]
			marked = marked[newline+1:]
		} else {
			marked = ""
		}
		kind, id, marker := parseMarker(line)
		_, knownMarker := locations[id]
		if !marker || !knownMarker {
			output.WriteString(line)
			continue
		}
		if kind == "start" {
			starts[id] = output.Len()
			continue
		}
		start, found := starts[id]
		location, known := locations[id]
		if found && known && output.Len() > start {
			recorder.mappings = append(recorder.mappings, offsetMapping{start: start, end: output.Len(), source: location})
		}
	}
	generated := output.String()
	result := recorder.Build(generated)
	sort.SliceStable(result.Mappings, func(i, j int) bool {
		if result.Mappings[i].Generated.Start.Offset != result.Mappings[j].Generated.Start.Offset {
			return result.Mappings[i].Generated.Start.Offset < result.Mappings[j].Generated.Start.Offset
		}
		return result.Mappings[i].Generated.End.Offset < result.Mappings[j].Generated.End.Offset
	})
	return generated, result
}

func IsMarkerLine(line string) bool {
	_, _, marker := parseMarker(line)
	return marker
}

// WithGoLineDirectives projects the shared map into Go's compiler-recognized
// line directives. Delve then sees TypeRB source paths without needing a
// TypeRB-specific breakpoint or stack-frame translation layer.
func WithGoLineDirectives(output, generatedPath string, mapping Map) string {
	if len(mapping.Mappings) == 0 {
		return output
	}
	lines := strings.SplitAfter(output, "\n")
	var result strings.Builder
	previousMapped := false
	for index, line := range lines {
		if line == "" && index == len(lines)-1 {
			continue
		}
		lineNumber := index + 1
		location, found := sourceForGeneratedLine(mapping, lineNumber)
		if found {
			sourceLine := location.Source.Span.Start.Line + lineNumber - location.Generated.Start.Line
			if end := location.Source.Span.End.Line; end > 0 && sourceLine > end {
				sourceLine = end
			}
			column := 1
			if lineNumber == location.Generated.Start.Line && location.Source.Span.Start.Column > 0 {
				column = location.Source.Span.Start.Column
			}
			fmt.Fprintf(&result, "//line %s:%d:%d\n", location.Source.Path, sourceLine, column)
			previousMapped = true
		} else if previousMapped {
			fmt.Fprintf(&result, "//line %s:%d:1\n", generatedPath, lineNumber)
			previousMapped = false
		}
		result.WriteString(line)
	}
	return result.String()
}

func sourceForGeneratedLine(mapping Map, line int) (Mapping, bool) {
	best := Mapping{}
	found := false
	bestWidth := 0
	for _, candidate := range mapping.Mappings {
		if line < candidate.Generated.Start.Line || line > candidate.Generated.End.Line {
			continue
		}
		if line == candidate.Generated.End.Line && candidate.Generated.End.Column <= 1 {
			continue
		}
		width := candidate.Generated.End.Offset - candidate.Generated.Start.Offset
		if !found || width < bestWidth {
			best = candidate
			bestWidth = width
			found = true
		}
	}
	return best, found
}

func parseMarker(line string) (string, int, bool) {
	trimmed := strings.TrimSpace(line)
	prefix := "// " + markerPrefix
	if !strings.HasPrefix(trimmed, prefix) {
		return "", 0, false
	}
	remainder := strings.TrimPrefix(trimmed, prefix)
	for _, kind := range []string{"start", "end"} {
		value := strings.TrimPrefix(remainder, kind+"_")
		if value == remainder {
			continue
		}
		id, err := strconv.Atoi(value)
		return kind, id, err == nil
	}
	return "", 0, false
}
