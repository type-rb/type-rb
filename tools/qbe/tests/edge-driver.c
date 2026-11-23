/* Diagnostic driver for an external QBE operation, not Native implementation. */
#include <fenv.h>
#include <inttypes.h>
#include <math.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

#pragma STDC FENV_ACCESS ON

extern double probe_d(double);
extern float probe_s(float);
extern double constant_root(void);
extern double memory_root(const double *);

static double (*volatile baseline_d)(double) = sqrt;
static float (*volatile baseline_s)(float) = sqrtf;
static uint64_t seed = UINT64_C(0x875ced812a71f593);
static uint64_t next(void) {
	seed ^= seed << 13;
	seed ^= seed >> 7;
	seed ^= seed << 17;
	return seed;
}

int main(void) {
	const uint64_t edges[] = {
		0, UINT64_C(0x8000000000000000), 1, UINT64_C(0x000fffffffffffff),
		UINT64_C(0x0010000000000000), UINT64_C(0x3ff0000000000000),
		UINT64_C(0x3fffffffffffffff), UINT64_C(0x4000000000000000),
		UINT64_C(0x4000000000000001), UINT64_C(0x4010000000000000),
		UINT64_C(0x7fefffffffffffff), UINT64_C(0x7ff0000000000000),
		UINT64_C(0xfff0000000000000), UINT64_C(0xbff0000000000000),
		UINT64_C(0x7ff8000000001234), UINT64_C(0x7ff0000000001234),
		UINT64_C(0xfff8000000005678), UINT64_C(0x8000000000000001)
	};
	const uint32_t edges_s[] = {0, 0x80000000, 1, 0x007fffff, 0x00800000,
		0x3f800000, 0x3fffffff, 0x40000000, 0x40000001, 0x40800000,
		0x7f7fffff, 0x7f800000, 0xff800000, 0xbf800000,
		0x7fc01234, 0x7f801234, 0xffc05678, 0x80000001};
	const int modes[] = {FE_TONEAREST, FE_DOWNWARD, FE_UPWARD, FE_TOWARDZERO};
	unsigned mismatches = 0, nan_payload_differences = 0, flag_differences = 0;
	unsigned cases = 0;
	for (unsigned m = 0; m < 4; ++m) {
		if (fesetround(modes[m])) return 2;
		for (unsigned i = 0; i < 100018; ++i) {
			uint64_t bits = i < 18 ? edges[i] : next();
			double x, expected, actual;
			memcpy(&x, &bits, sizeof x);
			feclearexcept(FE_ALL_EXCEPT);
			expected = baseline_d(x);
			int expected_flags = fetestexcept(FE_ALL_EXCEPT);
			feclearexcept(FE_ALL_EXCEPT);
			actual = probe_d(x);
			int actual_flags = fetestexcept(FE_ALL_EXCEPT);
			uint64_t e, a;
			memcpy(&e, &expected, sizeof e); memcpy(&a, &actual, sizeof a);
			if (isnan(expected) && isnan(actual)) nan_payload_differences += e != a;
			else if (e != a) {
				if (mismatches < 8) printf("mismatch,d,%u,%016" PRIx64 ",%016" PRIx64 ",%016" PRIx64 "\n", m, bits, e, a);
				mismatches++;
			}
			flag_differences += expected_flags != actual_flags;
			uint32_t bits_s = i < 18 ? edges_s[i] : (uint32_t)next();
			float xs, es, as;
			memcpy(&xs, &bits_s, sizeof xs);
			feclearexcept(FE_ALL_EXCEPT); es = baseline_s(xs);
			expected_flags = fetestexcept(FE_ALL_EXCEPT);
			feclearexcept(FE_ALL_EXCEPT); as = probe_s(xs);
			actual_flags = fetestexcept(FE_ALL_EXCEPT);
			uint32_t eb, ab;
			memcpy(&eb, &es, sizeof eb); memcpy(&ab, &as, sizeof ab);
			if (isnan(es) && isnan(as)) nan_payload_differences += eb != ab;
			else if (eb != ab) {
				if (mismatches < 8) printf("mismatch,s,%u,%08" PRIx32 ",%08" PRIx32 ",%08" PRIx32 "\n", m, bits_s, eb, ab);
				mismatches++;
			}
			flag_differences += expected_flags != actual_flags;
			cases += 2;
		}
	}
	if (fesetround(FE_TONEAREST)) return 2;
	double nine = 9.0;
	if (constant_root() != 3.0 || memory_root(&nine) != 3.0) return 3;
	printf("cases=%u mismatches=%u nan_payload_differences=%u flag_differences=%u\n",
		cases, mismatches, nan_payload_differences, flag_differences);
	return mismatches || flag_differences;
}
