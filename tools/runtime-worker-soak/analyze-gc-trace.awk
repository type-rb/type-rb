BEGIN {
	FS = ","
	prefix = "type-rb-native-gc-trace-v1"
	expected[1] = "collection"
	expected[2] = "automatic"
	expected[3] = "live-bytes"
	expected[4] = "next-target-bytes"
	expected[5] = "root-count"
	expected[6] = "root-capacity"
}

$1 == prefix {
	if (NF != 3 || $3 !~ /^[0-9]+$/) {
		print "invalid GC trace line at input line " NR > "/dev/stderr"
		exit 2
	}
	position += 1
	if ($2 != expected[position]) {
		print "unexpected GC trace field at input line " NR > "/dev/stderr"
		exit 2
	}
	value[position] = $3 + 0
	trace_lines += 1
	if (position != 6) {
		next
	}

	observation_count += 1
	collection = value[1]
	automatic = value[2]
	live = value[3]
	target = value[4]
	root_count = value[5]
	root_capacity = value[6]
	if (collection <= previous_collection) {
		print "GC trace collections are not strictly increasing" > "/dev/stderr"
		exit 2
	}
	if (automatic != 0 && automatic != 1) {
		print "GC trace automatic role is not boolean" > "/dev/stderr"
		exit 2
	}
	if (target == 0) {
		print "GC trace target is zero" > "/dev/stderr"
		exit 2
	}
	if (root_capacity != 64 || root_count > root_capacity) {
		print "GC trace root storage differs from the registered 64-word bound" > "/dev/stderr"
		exit 2
	}
	if (live > maximum_live_bytes) {
		maximum_live_bytes = live
	}
	if (root_count > maximum_root_count) {
		maximum_root_count = root_count
	}
	if (automatic == 1) {
		if (manual_observation_count != 0) {
			print "automatic GC trace follows the final manual observation" > "/dev/stderr"
			exit 2
		}
		if (collection % 64 != 0) {
			print "automatic GC trace is outside the 64-collection sampling interval" > "/dev/stderr"
			exit 2
		}
		if (automatic_observation_count == 0 && collection != 64) {
			print "first automatic GC trace is not collection 64" > "/dev/stderr"
			exit 2
		}
		if (automatic_observation_count != 0 && collection != previous_automatic_collection + 64) {
			print "automatic GC trace skipped a sampling interval" > "/dev/stderr"
			exit 2
		}
		if (live > 131072) {
			print "sampled post-sweep live heap exceeds 128 KiB" > "/dev/stderr"
			exit 2
		}
		automatic_observation_count += 1
		previous_automatic_collection = collection
	} else {
		manual_observation_count += 1
		if (manual_observation_count != 1 || live != 0 || root_count != 0) {
			print "final manual GC trace does not close the managed heap" > "/dev/stderr"
			exit 2
		}
	}
	previous_collection = collection
	final_collection = collection
	final_automatic = automatic
	position = 0
}

END {
	if (position != 0 || trace_lines != observation_count * 6) {
		print "incomplete GC trace observation" > "/dev/stderr"
		exit 2
	}
	if (observation_count < minimum_observations + 0) {
		print "insufficient GC trace observations" > "/dev/stderr"
		exit 2
	}
	if (manual_observation_count != 1 || final_automatic != 0) {
		print "GC trace is missing its final manual observation" > "/dev/stderr"
		exit 2
	}
	printf "observation_count=%d\n", observation_count
	printf "automatic_observation_count=%d\n", automatic_observation_count
	printf "manual_observation_count=%d\n", manual_observation_count
	printf "final_collection=%d\n", final_collection
	printf "maximum_sampled_live_bytes=%d\n", maximum_live_bytes
	printf "maximum_root_count=%d\n", maximum_root_count
	printf "root_capacity_words=64\n"
}
