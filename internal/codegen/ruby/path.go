package ruby

func pathJoinExpression(parent, child, windows string) string {
	return `->(parent, child, windows) {
		separator = windows ? "\\" : "/"
		child = child.gsub("/", "\\\\") if windows
		drive = windows && parent.match?(/\A[A-Za-z]:\z/)
		if parent.empty? || drive || parent.end_with?("/") || (windows && parent.end_with?("\\"))
			parent + child
		else
			parent + separator + child
		end
	}.call(` + parent + ", " + child + ", " + windows + ")"
}
