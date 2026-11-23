function executable_path(line,    prefix, remainder, ending) {
	prefix = index(line, "execve(\"")
	if (prefix == 0) return ""
	remainder = substr(line, prefix + length("execve(\""))
	ending = index(remainder, "\"")
	if (ending == 0) return ""
	return substr(remainder, 1, ending - 1)
}

{
	process_id = $1
	path = executable_path($0)
	if (path != "") {
		if ($0 ~ /= 0$/) {
			print path
		} else if ($0 ~ /<unfinished [.][.][.]>$/) {
			pending[process_id] = path
		}
		next
	}
	if ($0 ~ /<[^>]*execve resumed>/) {
		if ($0 ~ /= 0$/ && process_id in pending) print pending[process_id]
		delete pending[process_id]
	}
}
