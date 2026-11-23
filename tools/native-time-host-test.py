#!/usr/bin/env python3
"""Execute the actual QBE time adapters against the target's libc ABI."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import tempfile

DRIVER = r'''
#define _POSIX_C_SOURCE 200809L
#include <assert.h>
#include <errno.h>
#include <stdint.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/resource.h>
#include <sys/utsname.h>
#include <time.h>
#include <unistd.h>
struct string { int64_t kind, length, points; unsigned char data[]; };
struct array { int64_t kind, length, capacity; void *data; };
int64_t trbn_gc_temp_count=7;
static void *allocations[2048];
static size_t allocated;
void trbn_gc_temp_push(void *value) { assert(value); ++trbn_gc_temp_count; }
void *trbn_string_alloc(int64_t size) {
 assert(size>=17 && size<10485800 && allocated<2048);
 void *value=calloc(1,(size_t)size+8); assert(value);
 allocations[allocated++]=value; return value;
}
void *trbn_string_from_c(const char *value) {
 size_t length=strlen(value);
 struct string *s=trbn_string_alloc((int64_t)length+17);
 s->length=s->points=(int64_t)length; memcpy(s->data,value,length+1); return s;
}
extern int64_t trbn_time_zone_data(struct string *,int64_t,int64_t,struct array *);
extern int64_t trbn_time_zone_environment(struct array *);
extern int64_t trbn_time_clock(struct array *);
extern int64_t trbn_deadline_now(void);
void trbn_fail(const char *message,int64_t size) {
 fwrite(message,1,(size_t)size,stderr); abort();
}
static void check_string(struct string *value,const void *expected,size_t length) {
 assert(value && value->length==(int64_t)length);
 assert(!memcmp(value->data,expected,length)); assert(value->data[length]==0);
}
static void environment(const char *value,int present) {
 if(present) assert(!setenv("TZ",value,1)); else assert(!unsetenv("TZ"));
 struct string *values[2]={NULL,NULL}; struct array parts={0,2,2,values};
 assert(trbn_time_zone_environment(&parts)==present); assert(trbn_gc_temp_count==7);
 check_string(values[0],present?value:"",present?strlen(value):0);
 check_string(values[1],"archive.zip",11);
}
int main(void) {
 _Static_assert(sizeof(void *)==8 && sizeof(long)==8 && sizeof(off_t)==8,"LP64");
 _Static_assert(sizeof(struct timespec)==16,"timespec layout");
 _Static_assert(offsetof(struct timespec,tv_sec)==0 && offsetof(struct timespec,tv_nsec)==8,"timespec offsets");
 _Static_assert(sizeof(struct utsname)<=2048 && offsetof(struct utsname,sysname)==0,"bounded uname layout");
 _Static_assert(sizeof(((struct utsname *)0)->sysname)>=7,"uname system name");
#ifdef __APPLE__
 _Static_assert(CLOCK_MONOTONIC==6,"Darwin monotonic clock");
#elif defined(__linux__)
 _Static_assert(CLOCK_MONOTONIC==1,"Linux monotonic clock");
#else
#error unsupported monotonic clock profile
#endif
 _Static_assert(SEEK_SET==0 && SEEK_END==2 && CLOCK_REALTIME==0,"host constants");
 _Static_assert(ENOENT==2 && EINVAL==22,"host errors");
 assert(!setenv("ZONEINFO","archive.zip",1));
 environment("",0); environment("",1); environment("UTC",1); environment("Example/Zone",1);
 int64_t clock_values[2]={0,0}; struct array clock_parts={0,2,2,clock_values};
 assert(trbn_time_clock(&clock_parts)==1);
 assert(clock_values[0]>1000000000 && clock_values[1]>=0 && clock_values[1]<1000000000);
 struct timespec before,after;
 assert(!clock_gettime(CLOCK_MONOTONIC,&before));
 int64_t monotonic=trbn_deadline_now();
 assert(!clock_gettime(CLOCK_MONOTONIC,&after));
 assert(monotonic>=before.tv_sec*INT64_C(1000000000)+before.tv_nsec);
 assert(monotonic<=after.tv_sec*INT64_C(1000000000)+after.tv_nsec);
 char filename[]="time-host-XXXXXX"; int fd=mkstemp(filename); assert(fd>=0);
 const unsigned char bytes[]={65,0,66,255,67}; assert(write(fd,bytes,sizeof(bytes))==5); assert(!close(fd));
 struct string *path=trbn_string_from_c(filename);
 struct string *values[2]={NULL,NULL}; struct array parts={0,2,2,values};
 assert(trbn_time_zone_data(path,0,-1,&parts)==0); check_string(values[0],bytes,5);
 assert(trbn_time_zone_data(path,1,3,&parts)==0); check_string(values[0],bytes+1,3);
 assert(trbn_time_zone_data(path,-2,2,&parts)==0); check_string(values[0],bytes+3,2);
 assert(trbn_time_zone_data(path,5,0,&parts)==0); check_string(values[0],"",0);
 assert(trbn_time_zone_data(path,6,0,&parts)==-2); check_string(values[0],"",0);
 assert(trbn_time_zone_data(path,-6,1,&parts)==-2);
 assert(trbn_time_zone_data(path,0,6,&parts)==-2);
 assert(trbn_time_zone_data(path,0,-2,&parts)==-2);
 assert(trbn_gc_temp_count==7);
 struct rlimit limit; assert(!getrlimit(RLIMIT_NOFILE,&limit));
 struct rlimit small=limit; if(small.rlim_cur>64)small.rlim_cur=64; assert(!setrlimit(RLIMIT_NOFILE,&small));
 for(int i=0;i<128;i++) {
  assert(trbn_time_zone_data(path,0,-1,&parts)==0); check_string(values[0],bytes,5);
  assert(trbn_time_zone_data(path,9,4,&parts)==-2); assert(trbn_gc_temp_count==7);
 }
 assert(!setrlimit(RLIMIT_NOFILE,&limit));
 FILE *large=fopen(filename,"wb"); assert(large); assert(!ftruncate(fileno(large),10485761)); assert(!fclose(large));
 assert(trbn_time_zone_data(path,0,-1,&parts)==-1); check_string(values[0],"",0);
 assert(trbn_time_zone_data(path,-22,22,&parts)==0); assert(values[0]->length==22);
 assert(!unlink(filename));
 assert(trbn_time_zone_data(path,0,-1,&parts)==ENOENT); assert(values[1]->length>0);
 struct string *nul=trbn_string_from_c("bad!path"); nul->data[3]=0;
 assert(trbn_time_zone_data(nul,0,-1,&parts)==EINVAL);
 struct array wrong={0,1,1,values};
 assert(trbn_time_zone_data(path,0,-1,&wrong)==EINVAL);
 assert(trbn_time_zone_environment(&wrong)==0); assert(trbn_time_clock(&wrong)==0);
 assert(trbn_gc_temp_count==7);
 for(size_t i=0;i<allocated;i++)free(allocations[i]);
 puts("PASS time host environment, range, binary bytes, errors, clock and descriptor lifetime");
 return 0;
}
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--qbe', required=True, type=Path)
    parser.add_argument('--receipt', required=True, type=Path)
    args = parser.parse_args()
    source = Path(__file__).resolve().parent.parent / 'compiler/src/backend/qbe/runtime/time_host.trb'
    deadline_source = source.with_name('deadlines.trb')
    pieces = []
    for line in source.read_text().splitlines():
        match = re.fullmatch(r'\s*qbe_output_line\(output, ("(?:[^"\\]|\\.)*")\)', line)
        if match:
            pieces.append(json.loads(match[1]))
    clock_body = deadline_source.read_text().split('def emit_deadline_clock_runtime(', 1)[1].split('\nend', 1)[0]
    for line in clock_body.splitlines():
        match = re.fullmatch(r'\s*qbe_output_line\(output, ("(?:[^"\\]|\\.)*")\)', line)
        if match:
            pieces.append(json.loads(match[1]))
    il = '\n'.join(pieces)
    for name in ['trbn_time_clock', 'trbn_time_zone_data', 'trbn_time_zone_environment', 'trbn_deadline_now']:
        marker = 'function l $' + name + '('
        if il.count(marker) != 1:
            raise ValueError('missing or duplicate actual adapter: ' + name)
        il = il.replace(marker, 'export ' + marker)
    receipt = {'schemaVersion': 1, 'platform': platform.platform(),
               'sourceSha256': hashlib.sha256(source.read_bytes()).hexdigest(),
               'deadlineSourceSha256': hashlib.sha256(deadline_source.read_bytes()).hexdigest(),
               'qbeSha256': hashlib.sha256(args.qbe.read_bytes()).hexdigest(),
               'checks': [], 'complete': False}

    def save():
        args.receipt.parent.mkdir(parents=True, exist_ok=True)
        args.receipt.write_text(json.dumps(receipt, indent=2) + '\n')

    try:
        with tempfile.TemporaryDirectory(prefix='native-time-host-') as directory:
            root = Path(directory)
            (root / 'runtime.ssa').write_text(il)
            (root / 'driver.c').write_text(DRIVER)
            commands = [[str(args.qbe.resolve()), '-o', str(root / 'runtime.s'), str(root / 'runtime.ssa')],
                        [os.environ.get('CC', 'cc'), '-std=c11', '-Wall', '-Wextra', '-Werror',
                         str(root / 'driver.c'), str(root / 'runtime.s'), '-o', str(root / 'probe')]
                        + (['-ldl'] if platform.system() == 'Linux' else []), [str(root / 'probe')]]
            for command in commands:
                process = subprocess.run(command, cwd=root, text=True, capture_output=True, timeout=60)
                receipt['checks'].append({'command': [s.replace(str(root), '<work>') for s in command],
                                          'code': process.returncode, 'stdout': process.stdout, 'stderr': process.stderr})
                save()
                if process.returncode:
                    raise ValueError('host adapter qualification failed: ' + process.stderr)
            receipt['complete'] = True
            print(receipt['checks'][-1]['stdout'].strip())
    except BaseException as error:
        receipt['error'] = type(error).__name__ + ': ' + str(error)
        raise
    finally:
        save()


if __name__ == '__main__':
    main()
