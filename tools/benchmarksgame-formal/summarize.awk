BEGIN {
	FS = "\t"
	OFS = "\t"
	expected_header = "phase\tround\tretained_index\torder\tcase\tlane\tcandidate\tverdict\treturnvalue\texitsignal\tterminationreason\twalltime_seconds\tcputime_seconds\tmemory_bytes"
	candidate_count = split("typerb-native typerb-go c cpp go rust java", candidates, " ")
}

NR == 1 {
	if ($0 != expected_header) exit 2
	next
}

{
	if (NF != 14) exit 2
	if (case_name == "") {
		case_name = $5
		lane = $6
	}
	if ($5 != case_name || $6 != lane) exit 2
	seen[$7] = 1
	if ($1 != "retained") next
	total[$7] += 1
	if ($8 != "pass") next
	passed[$7] += 1
	sample_index = passed[$7]
	wall[$7, sample_index] = $12 + 0
	cpu[$7, sample_index] = $13 + 0
	memory[$7, sample_index] = $14 + 0
}

function metric_value(candidate, metric, sample) {
	if (metric == "wall") return wall[candidate, sample]
	if (metric == "cpu") return cpu[candidate, sample]
	return memory[candidate, sample]
}

function median(candidate, metric, count,    sample, position, value, result) {
	for (sample = 1; sample <= count; sample += 1) {
		value = metric_value(candidate, metric, sample)
		position = sample
		while (position > 1 && sorted[position - 1] > value) {
			sorted[position] = sorted[position - 1]
			position -= 1
		}
		sorted[position] = value
	}
	result = sorted[int((count + 1) / 2)]
	for (sample = 1; sample <= count; sample += 1) delete sorted[sample]
	return result
}

END {
	if (NR < 2) exit 2
	print "case", "lane", "candidate", "retained", "passed", "walltime_median_seconds", "cputime_median_seconds", "memory_median_bytes", "status"
	for (candidate_index = 1; candidate_index <= candidate_count; candidate_index += 1) {
		candidate = candidates[candidate_index]
		if (!(candidate in seen) || total[candidate] != 11 || passed[candidate] != 11) {
			print case_name, lane, candidate, total[candidate] + 0, passed[candidate] + 0, "", "", "", "incomplete"
			continue
		}
		print case_name, lane, candidate, total[candidate], passed[candidate], \
			median(candidate, "wall", passed[candidate]), \
			median(candidate, "cpu", passed[candidate]), \
			median(candidate, "memory", passed[candidate]), \
			"pass"
	}
}
