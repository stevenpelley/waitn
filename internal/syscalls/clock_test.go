package syscalls

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// experiment to determine which linux clock is used in setting /proc/<pid>/stat starttime
func TestProcessStarttimeClocks(t *testing.T) {
	require := require.New(t)
	// get starttime of the current process
	pid := os.Getpid()
	// read stat
	bytes, err := os.ReadFile(path.Join("/proc", fmt.Sprintf("%v", pid), "stat"))
	require.NoError(err)
	stat := string(bytes)
	fields := strings.Fields(stat)
	// see man proc, search /proc/<pid>/stat
	// starttime will be "ticks" of USER_HZ/CLK_TCK since boot
	require.GreaterOrEqual(len(fields), 22)
	starttimeStr := fields[21]
	starttime, err := strconv.Atoi(starttimeStr)
	require.NoError(err)

	fmt.Printf("starttime: %v\n", starttime)

	// now read system clock in several ways
	// these all appear to provide the same time for me
	printtime(unix.CLOCK_MONOTONIC, "CLOCK_MONOTONIC", require)
	printtime(unix.CLOCK_MONOTONIC_RAW, "CLOCK_MONOTONIC_RAW", require)
	printtime(unix.CLOCK_BOOTTIME, "CLOCK_BOOTTIME", require)
	// do again to see that they each increase monotonically _with each other_
	printtime(unix.CLOCK_MONOTONIC, "CLOCK_MONOTONIC", require)
	printtime(unix.CLOCK_MONOTONIC_RAW, "CLOCK_MONOTONIC_RAW", require)
	printtime(unix.CLOCK_BOOTTIME, "CLOCK_BOOTTIME", require)

	// what is USER_HZ?
	// "see sysconf(_SC_CLK_TCK))"
	bytes, err = exec.Command("getconf", "CLK_TCK").Output()
	require.NoError(err)
	fmt.Printf("CLK_TCK: %v", string(bytes))

	// looking at linux source the process PCB process control block has both start_time and start_boottime.
	// these resolve to timekeeping.c ktime_get and ktime_get_with_offset, respectively.
}

func gettime(clock int32) (unix.Timespec, error) {
	var timespec unix.Timespec
	err := unix.ClockGettime(clock, &timespec)
	return timespec, err
}

func printtime(clock int32, clockName string, require *require.Assertions) {
	time, err := gettime(unix.CLOCK_MONOTONIC)
	require.NoError(err)
	fmt.Printf("%v: %v\n", clockName, time)
}
