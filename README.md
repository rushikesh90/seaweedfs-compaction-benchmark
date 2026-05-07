# SeaweedFS Compaction Benchmark

This repository contains a benchmark that simulates SeaweedFS-style volume compaction, comparing two approaches:

1. **Traditional userspace copy**: `read()` → Go buffer → `write()`
2. **Zero-copy sendfile()**: Kernel-to-kernel copy using the `sendfile()` system call

## Overview

The benchmark:
- Generates a large test volume file
- Creates live regions (simulating active data in a SeaweedFS volume)
- Compacts only the live regions (skipping deleted space)
- Measures performance metrics:
  - Throughput (MB/s)
  - CPU time
  - Wall clock time
  - Memory allocations
  - GC activity

## Files

- `seaweedfs_like_compaction_benchmark.go` - The Go source code implementing the benchmark
- `go.mod` - Go module definition
- `go.sum` - Go module checksums
- `compact_bench` - Compiled benchmark executable

## Building

```bash
go build -o compact_bench seaweedfs_like_compaction_benchmark.go
```

## Running

```bash
./compact_bench
```

### Options

- `-size-gb N`: Specify volume size in GB (default: 1)
- For cold-cache testing: `sudo sh -c 'echo 3 > /proc/sys/vm/drop_caches'`

### Profiling

- Observe sendfile syscalls: `strace -e sendfile ./compact_bench`
- Observe CPU usage: `perf stat ./compact_bench`

## Benchmark Results

Sample output from running the benchmark:

```
Creating live regions map...
Live regions: 176
Live percentage: 70%

=== USERSpace read/write compaction ===
BEFORE HeapAlloc=4 MB HeapSys=7 MB NumGC=1
AFTER  HeapAlloc=4 MB HeapSys=7 MB NumGC=2

RESULTS
-----------------------------------
Method           : read/write
Data copied      : 704.00 MB
Wall time        : 16.51 sec
CPU time         : 0.99 sec
Throughput       : 42.64 MB/s
CPU per GB       : 1.44 sec
-----------------------------------

=== SENDFILE compaction ===
BEFORE HeapAlloc=0 MB HeapSys=7 MB NumGC=3
AFTER  HeapAlloc=0 MB HeapSys=7 MB NumGC=3

RESULTS
-----------------------------------
Method           : sendfile
Data copied      : 704.00 MB
Wall time        : 12.09 sec
CPU time         : 0.70 sec
Throughput       : 58.23 MB/s
CPU per GB       : 1.02 sec
-----------------------------------

DONE
```

## Key Findings

The benchmark demonstrates that `sendfile()` provides significant performance advantages over traditional read/write loops:
- **Higher throughput**: 58.23 MB/s vs 42.64 MB/s (~36% improvement)
- **Lower CPU usage**: 0.70 sec vs 0.99 sec (~29% less CPU time)
- **Better CPU efficiency**: 1.02 sec/GB vs 1.44 sec/GB

This shows that zero-copy techniques like `sendfile()` can substantially improve storage compaction performance in systems like SeaweedFS by reducing data copying between kernel and user space.

## Implementation Details

The benchmark simulates SeaweedFS volume compaction by:
1. Creating a large file filled with random data (simulating a volume)
2. Marking 70% of 4MB blocks as "live" (active data)
3. Copying only the live regions to a new file (compaction)
4. Comparing performance between traditional I/O and sendfile() approaches

This mirrors how SeaweedFS can reclaim space by copying only active data blocks to new volumes during the compaction process.
