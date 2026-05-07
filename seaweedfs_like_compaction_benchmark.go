//
// seaweedfs_like_compaction_benchmark.go
//
// Simulates SeaweedFS-style volume compaction.
//
// Two approaches:
//
//   1. Traditional userspace copy
//      read() -> Go buffer -> write()
//
//   2. sendfile()
//      kernel -> kernel copy
//
// The benchmark:
//
//   - Generates a large "volume file"
//   - Creates "live regions"
//   - Compacts only live regions
//   - Measures:
//       * throughput
//       * CPU time
//       * allocations
//       * GC activity
//       * wall clock
//
// Build:
//   go build -o compact_bench seaweedfs_like_compaction_benchmark.go
//
// Run:
//   ./compact_bench
//
// Optional:
//   ./compact_bench -size-gb 4
//
// Optional cold-cache test:
//   sudo sh -c 'echo 3 > /proc/sys/vm/drop_caches'
//
// Observe sendfile syscalls:
//   strace -e sendfile ./compact_bench
//
// Observe CPU:
//   perf stat ./compact_bench
//

package main

import (
cryptoRand "crypto/rand"
"flag"
"fmt"
"io"
"math/rand"
"os"
"runtime"
"syscall"
"time"

"golang.org/x/sys/unix"
)

const (
BlockSize      = 4 * 1024 * 1024 // 4MB
LivePercentage = 70
)

type Region struct {
Offset int64
Size   int64
}

func must(err error) {
if err != nil {
panic(err)
}
}

func generateVolume(path string, sizeGB int) {
fmt.Println("Generating test volume...")

size := int64(sizeGB) * 1024 * 1024 * 1024

f, err := os.Create(path)
must(err)
defer f.Close()

buf := make([]byte, BlockSize)

written := int64(0)

start := time.Now()

for written < size {

_, err := cryptoRand.Read(buf)
must(err)

n, err := f.Write(buf)
must(err)

written += int64(n)

percent := float64(written) / float64(size) * 100

fmt.Printf("\rProgress: %.1f%%", percent)
}

fmt.Println()

elapsed := time.Since(start)

fmt.Printf(
"Generated %d GB in %.2f sec\n",
sizeGB,
elapsed.Seconds(),
)
}

func createRegions(fileSize int64) []Region {

var regions []Region

blockCount := fileSize / BlockSize

for i := int64(0); i < blockCount; i++ {

if rand.Intn(100) < LivePercentage {

regions = append(regions, Region{
Offset: i * BlockSize,
Size:   BlockSize,
})
}
}

return regions
}

func printMemStats(prefix string) {

var m runtime.MemStats

runtime.ReadMemStats(&m)

fmt.Printf(
"%s HeapAlloc=%d MB HeapSys=%d MB NumGC=%d\n",
prefix,
m.HeapAlloc/1024/1024,
m.HeapSys/1024/1024,
m.NumGC,
)
}

func compactReadWrite(srcPath, dstPath string, regions []Region) {

fmt.Println("\n=== USERSpace read/write compaction ===")

src, err := os.Open(srcPath)
must(err)
defer src.Close()

dst, err := os.Create(dstPath)
must(err)
defer dst.Close()

buf := make([]byte, BlockSize)

startCPU := getCPUTime()

start := time.Now()

printMemStats("BEFORE")

var totalCopied int64

for _, r := range regions {

_, err := src.Seek(r.Offset, io.SeekStart)
must(err)

remaining := r.Size

for remaining > 0 {

chunk := int64(len(buf))

if remaining < chunk {
chunk = remaining
}

n, err := io.ReadFull(src, buf[:chunk])

if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
panic(err)
}

_, err = dst.Write(buf[:n])
must(err)

remaining -= int64(n)

totalCopied += int64(n)
}
}

dst.Sync()

elapsed := time.Since(start)

endCPU := getCPUTime()

printMemStats("AFTER ")

printResults(
"read/write",
totalCopied,
elapsed,
endCPU-startCPU,
)
}

func compactSendfile(srcPath, dstPath string, regions []Region) {

fmt.Println("\n=== SENDFILE compaction ===")

src, err := os.Open(srcPath)
must(err)
defer src.Close()

dst, err := os.Create(dstPath)
must(err)
defer dst.Close()

startCPU := getCPUTime()

start := time.Now()

printMemStats("BEFORE")

var totalCopied int64

for _, r := range regions {

offset := r.Offset
remaining := int(r.Size)

for remaining > 0 {

n, err := unix.Sendfile(
int(dst.Fd()),
int(src.Fd()),
&offset,
remaining,
)

if err != nil {
panic(err)
}

if n == 0 {
break
}

remaining -= n

totalCopied += int64(n)
}
}

dst.Sync()

elapsed := time.Since(start)

endCPU := getCPUTime()

printMemStats("AFTER ")

printResults(
"sendfile",
totalCopied,
elapsed,
endCPU-startCPU,
)
}

func getCPUTime() float64 {

var r syscall.Rusage

syscall.Getrusage(syscall.RUSAGE_SELF, &r)

user :=
float64(r.Utime.Sec) +
float64(r.Utime.Usec)/1e6

sys :=
float64(r.Stime.Sec) +
float64(r.Stime.Usec)/1e6

return user + sys
}

func printResults(
name string,
bytes int64,
elapsed time.Duration,
cpuSeconds float64,
) {

mb := float64(bytes) / 1024 / 1024

throughput := mb / elapsed.Seconds()

fmt.Println("\nRESULTS")
fmt.Println("-----------------------------------")
fmt.Printf("Method           : %s\n", name)
fmt.Printf("Data copied      : %.2f MB\n", mb)
fmt.Printf("Wall time        : %.2f sec\n", elapsed.Seconds())
fmt.Printf("CPU time         : %.2f sec\n", cpuSeconds)
fmt.Printf("Throughput       : %.2f MB/s\n", throughput)
fmt.Printf(
"CPU per GB       : %.2f sec\n",
cpuSeconds/(mb/1024),
)
fmt.Println("-----------------------------------")
}

func main() {

sizeGB := flag.Int(
"size-gb",
1,
"volume size in GB",
)

flag.Parse()

rand.Seed(time.Now().UnixNano())

volume := "volume.dat"

info, err := os.Stat(volume)

if os.IsNotExist(err) {

generateVolume(volume, *sizeGB)

info, err = os.Stat(volume)
must(err)
}

fmt.Println("\nCreating live regions map...")

regions := createRegions(info.Size())

fmt.Printf(
"Live regions: %d\n",
len(regions),
)

fmt.Printf(
"Live percentage: %d%%\n",
LivePercentage,
)

runtime.GC()

compactReadWrite(
volume,
"compacted_rw.dat",
regions,
)

runtime.GC()

compactSendfile(
volume,
"compacted_sf.dat",
regions,
)

fmt.Println("\nDONE")
}
