#!/usr/bin/env python3
"""Exercise recursive test comparison with a valid pointer whose low word is zero."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile


DRIVER = r'''
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/mman.h>
#include <unistd.h>
extern int64_t trbn_test_pair_seen(void *, int64_t, int64_t, int64_t);
struct pair { struct pair *parent; int64_t type, left, right; };
static int check(struct pair *head, int64_t type, int64_t left, int64_t right,
                 int64_t expected) {
 int64_t actual=trbn_test_pair_seen(head,type,left,right);
 if(actual==expected)return 0;
 fprintf(stderr,"expected %lld, received %lld\n",(long long)expected,(long long)actual);
 return 1;
}
int main(void) {
 /* Reserve address space and commit only one page owned by this mapping.
  * No fixed mapping or physical 4 GiB allocation is used. */
 const size_t boundary=(size_t)UINT64_C(1)<<32;
 const size_t page=(size_t)sysconf(_SC_PAGESIZE);
 const size_t reservation=boundary+page;
 void *mapping=mmap(NULL,reservation,PROT_NONE,MAP_PRIVATE|MAP_ANON,-1,0);
 if(mapping==MAP_FAILED){perror("reserve pointer boundary");return 77;}
 uintptr_t address=((uintptr_t)mapping+boundary-1)&~(boundary-1);
 if(!address || (uint32_t)address || mprotect((void *)address,page,PROT_READ|PROT_WRITE)) {
  perror("commit pointer boundary");munmap(mapping,reservation);return 77;
 }
 struct pair *wide=(struct pair *)address;
 *wide=(struct pair){NULL,7,11,13};
 struct pair outer={wide,8,17,19};
 int failed=check(NULL,7,11,13,0);
 failed+=check(wide,7,11,13,1);
 failed+=check(wide,8,11,13,0);
 failed+=check(wide,7,17,13,0);
 failed+=check(wide,7,11,19,0);
 failed+=check(&outer,7,11,13,1);
 failed+=check(&outer,8,17,19,1);
 failed+=check(&outer,7,17,19,0);
 if(munmap(mapping,reservation)){perror("release pointer boundary");return 77;}
 if(failed)return 1;
 puts("PASS 8 recursive comparison pointer-boundary controls");
 return 0;
}
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--qbe", type=Path, required=True)
    parser.add_argument("--source-root", type=Path)
    args = parser.parse_args()
    root = args.source_root or Path(__file__).resolve().parent.parent
    source = root / "compiler/src/backend/qbe/runtime/test_comparison.trb"
    il = "\n".join(json.loads(match[0]) for match in
                   re.finditer(r'"(?:[^"\\]|\\.)*"', source.read_text()))
    functions = re.findall(r"(?ms)^function[^\n]*\([^\n]*\) \{\n.*?^\}", il)
    pair = [code for code in functions if "$trbn_test_pair_seen(" in code.split("\n")[0]]
    if len(pair) != 1:
        raise ValueError("expected exactly one authored pair-visit runtime")
    with tempfile.TemporaryDirectory(prefix="native-test-comparison-") as directory:
        work = Path(directory)
        (work / "runtime.ssa").write_text("export " + pair[0] + "\n")
        (work / "driver.c").write_text(DRIVER)
        subprocess.run([str(args.qbe.resolve()), "-o", str(work / "runtime.s"),
                        str(work / "runtime.ssa")], check=True, timeout=30)
        subprocess.run(["cc", "-Wall", "-Wextra", "-Werror", str(work / "driver.c"),
                        str(work / "runtime.s"), "-o", str(work / "probe")],
                       check=True, timeout=30)
        subprocess.run([str(work / "probe")], check=True, timeout=10)


if __name__ == "__main__":
    main()
