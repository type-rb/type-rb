package repl

const (
	colorReset   = "\x1b[0m"
	colorTitle   = "\x1b[1;38;2;130;170;255m"
	colorInput   = "\x1b[1;38;2;214;222;235m"
	colorMuted   = "\x1b[38;2;98;114;164m"
	colorName    = "\x1b[38;2;197;228;120m"
	colorValue   = "\x1b[38;2;197;228;120m"
	colorType    = "\x1b[38;2;127;219;202m"
	colorSuccess = "\x1b[38;2;34;218;110m"
	colorError   = "\x1b[38;2;255;88;116m"
)

func colorize(enabled bool, color, value string) string {
	if !enabled {
		return value
	}
	return color + value + colorReset
}
