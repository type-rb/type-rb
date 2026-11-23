BEGIN {
	FS = ","
}

NR == 1 {
	if ($1 != "elapsed_seconds" || $2 != "rss_bytes" || $3 != "completed_phases" || $4 != "fd_count" || $5 != "thread_count") {
		print "invalid process-series header" > "/dev/stderr"
		exit 2
	}
	next
}

{
	if (NF != 5 || $1 !~ /^[0-9]+([.][0-9]+)?$/ || $2 !~ /^[0-9]+$/ || $3 !~ /^[0-9]+$/ || $4 !~ /^[0-9]+$/ || $5 !~ /^[0-9]+$/) {
		print "invalid process sample at line " NR > "/dev/stderr"
		exit 2
	}
	if ($4 + 0 == 0 || $5 + 0 == 0) {
		print "process sample was captured across the exit boundary at line " NR > "/dev/stderr"
		exit 2
	}
	all_count += 1
	if ($2 + 0 > maximum_rss) {
		maximum_rss = $2 + 0
	}
	if ($3 + 0 >= 6) {
		retained_count += 1
		elapsed[retained_count] = $1 + 0
		rss[retained_count] = $2 + 0
		fd = $4 + 0
		thread = $5 + 0
		if (retained_count == 1 || fd < minimum_fd) minimum_fd = fd
		if (retained_count == 1 || fd > maximum_fd) maximum_fd = fd
		if (retained_count == 1 || thread < minimum_thread) minimum_thread = thread
		if (retained_count == 1 || thread > maximum_thread) maximum_thread = thread
	}
}

END {
	if (all_count < 2 || retained_count < 8) {
		print "insufficient process samples" > "/dev/stderr"
		exit 2
	}

	quartile_count = int(retained_count / 4)
	for (sample_index = 1; sample_index <= quartile_count; sample_index += 1) {
		first_quartile[sample_index] = rss[sample_index]
		last_quartile[sample_index] = rss[retained_count - quartile_count + sample_index]
	}
	for (left = 1; left <= quartile_count; left += 1) {
		for (right = left + 1; right <= quartile_count; right += 1) {
			if (first_quartile[right] < first_quartile[left]) {
				temporary = first_quartile[left]
				first_quartile[left] = first_quartile[right]
				first_quartile[right] = temporary
			}
			if (last_quartile[right] < last_quartile[left]) {
				temporary = last_quartile[left]
				last_quartile[left] = last_quartile[right]
				last_quartile[right] = temporary
			}
		}
	}
	middle = int(quartile_count / 2)
	if (quartile_count % 2 == 0) {
		first_median = (first_quartile[middle] + first_quartile[middle + 1]) / 2
		last_median = (last_quartile[middle] + last_quartile[middle + 1]) / 2
	} else {
		first_median = first_quartile[middle + 1]
		last_median = last_quartile[middle + 1]
	}
	for (sample_index = 1; sample_index <= retained_count; sample_index += 1) {
		x = elapsed[sample_index]
		y = rss[sample_index]
		sum_x += x
		sum_y += y
		sum_xx += x * x
		sum_xy += x * y
	}
	denominator = retained_count * sum_xx - sum_x * sum_x
	if (denominator == 0) {
		print "process sample times have zero variance" > "/dev/stderr"
		exit 2
	}
	growth = last_median - first_median
	slope = (retained_count * sum_xy - sum_x * sum_y) / denominator * 60
	if (enforce_native + 0 == 1) {
		if (maximum_rss > 67108864 || growth > 8388608 || slope > 1048576) {
			print "Native process memory exceeds the registered bound" > "/dev/stderr"
			exit 2
		}
		if (minimum_fd != maximum_fd || minimum_thread != maximum_thread) {
			print "Native descriptors or threads grow after warmup" > "/dev/stderr"
			exit 2
		}
	}
	printf "sample_count=%d\n", all_count
	printf "post_warmup_sample_count=%d\n", retained_count
	printf "maximum_rss_bytes=%d\n", maximum_rss
	printf "first_quartile_median_rss_bytes=%.0f\n", first_median
	printf "last_quartile_median_rss_bytes=%.0f\n", last_median
	printf "quartile_median_growth_bytes=%.0f\n", growth
	printf "fitted_rss_slope_bytes_per_minute=%.6f\n", slope
	printf "minimum_fd_count=%d\n", minimum_fd
	printf "maximum_fd_count=%d\n", maximum_fd
	printf "minimum_thread_count=%d\n", minimum_thread
	printf "maximum_thread_count=%d\n", maximum_thread
}
