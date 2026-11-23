BEGIN {
	FS = "\t"
	OFS = "\t"
	expected_header = "phase\tround\tretained_index\torder\tcase\tcandidate\tverdict\treturnvalue\texitsignal\tterminationreason\twalltime_seconds\tcputime_seconds\tmemory_bytes\tartifact_bytes\tartifact_sha256\tprogram_status\tprogram_stderr_empty\tprogram_stdout_exact"
	candidate_count = split("typerb-native typerb-go", candidates, " ")
}

NR == 1 {
	if ($0 != expected_header) exit 2
	next
}

{
	if (NF != 18) exit 2
	if (case_name == "") case_name = $5
	if ($5 != case_name) exit 2
	seen[$6] = 1
	if ($1 != "retained") next
	total[$6] += 1
	if ($7 != "pass") next
	passed[$6] += 1
	sample_index = passed[$6]
	wall[$6, sample_index] = $11 + 0
	cpu[$6, sample_index] = $12 + 0
	memory[$6, sample_index] = $13 + 0
	artifact[$6, sample_index] = $14 + 0
	if (!(($6) in artifact_min) || $14 + 0 < artifact_min[$6]) artifact_min[$6] = $14 + 0
	if (!(($6) in artifact_max) || $14 + 0 > artifact_max[$6]) artifact_max[$6] = $14 + 0
	variant_key = $6 SUBSEP $14 SUBSEP $15
	if (!(variant_key in artifact_variant)) {
		artifact_variant[variant_key] = 1
		artifact_variant_count[$6] += 1
	}
	if (!(($6) in artifact_hash)) artifact_hash[$6] = $15
}

function metric_value(candidate, metric, sample) {
	if (metric == "wall") return wall[candidate, sample]
	if (metric == "cpu") return cpu[candidate, sample]
	if (metric == "artifact") return artifact[candidate, sample]
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
	print "case", "candidate", "retained", "passed", "walltime_median_seconds", "cputime_median_seconds", "memory_median_bytes", "artifact_bytes_median", "artifact_bytes_min", "artifact_bytes_max", "artifact_variant_count", "artifact_reproducible", "artifact_sha256", "status"
	for (candidate_index = 1; candidate_index <= candidate_count; candidate_index += 1) {
		candidate = candidates[candidate_index]
		if (!(candidate in seen) || total[candidate] != 11 || passed[candidate] != 11 || artifact_variant_count[candidate] < 1) {
			print case_name, candidate, total[candidate] + 0, passed[candidate] + 0, "", "", "", "", "", "", "", "", "", "incomplete"
			continue
		}
		artifact_reproducible = artifact_variant_count[candidate] == 1 ? "true" : "false"
		summary_hash = artifact_reproducible == "true" ? artifact_hash[candidate] : ""
		print case_name, candidate, total[candidate], passed[candidate], \
			median(candidate, "wall", passed[candidate]), \
			median(candidate, "cpu", passed[candidate]), \
			median(candidate, "memory", passed[candidate]), \
			median(candidate, "artifact", passed[candidate]), \
			artifact_min[candidate], artifact_max[candidate], \
			artifact_variant_count[candidate], artifact_reproducible, summary_hash, "pass"
	}
}
