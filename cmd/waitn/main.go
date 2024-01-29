package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stevenpelley/waitn/internal/syscalls"
	"golang.org/x/sys/unix"
)

func main() {
	var starttime uint64
	flag.Uint64Var(&starttime, "starttime", 0,
		`Provide a timestamp from clock_gettime's CLOCK_BOOTTIME clock, read after
	all provided processes were started.  Used to determine identity of
	processes assuming PIDs may be reused.  This time is in ns since system boot.`)

	var clkTck uint64
	flag.Uint64Var(&clkTck, "clk-tck", 100, "CLK_TCK system value.  See getconf CLK_TCK. Must be positive.")

	var errorOnUnknown bool
	flag.BoolVar(&errorOnUnknown, "error-on-unknown", false,
		"if any process cannot be found return an error code, not 0")

	var timeoutMs int64
	flag.Int64Var(&timeoutMs, "timeout", 0, "timeout in ms.  Negative implies no timeout.  Zero means to return immediately if no process is ready")

	flag.Usage = func() {
		fmt.Fprintf(
			flag.CommandLine.Output(),
			"waitn: wait for the first of several processes to terminate, as in Bash's wait -n.\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\n")
		fmt.Fprintf(
			flag.CommandLine.Output(),
			`arguments are <pid>[:starttime] where starttime is field 22 of /proc/<pid>/stat.
If provided always check identity of the process using provided starttime
otherwise check using --starttime if provided
otherwise trust that the pid names the correct process.
Pids and starttimes must be positive.

Finding processes.  A process with pid is "found" if pidfd_open returns a
process AND one of the following:
- --starttime is not provided  and the pid is given no starttime
- the pid is given a starttime and the value read in field 22 of
/proc/pid/stat after opening the pidfd matches the provided starttime
- the pid is not given a starttime, --starttime is provided and
is greater than the corresponding time found in field 22 of
/proc/pid/stat.  The stat starttime is converted by multiplying by
--clk-tck

a process that is not found, or that is found and completes, will have
its pid printed to stdout
all processes are searched for prior to determining if any process
completes (a process will never be determined to complete via pidfd and
returned when some other process is not found)

the returned pid is intended to be largely deterministic.  The first pid
to be not found is returned.  Only then the first process to complete is
returned.  Subsequent calls with the same list of pids should return the
same pid or some pid listed earlier (assuming no pid reuse)

NOTE WELL: pids may be reused; processes may alias.  If this happens a call to
waitn may block for the incorrect process with the same pid.

NOTE WELL: this utility uses Linux's pidfd to wait for non-child processes.  It
is up to you to ensure that the processes you wait for are visible to this call.
Otherwise you the utility may not find the process, and you may incorrectly
interpret it as having terminated.  It may also result in a panic.

return values:
0 - a process was found and completed
0 - a process was not found and presumably completed prior to this command
1 - --error-on-unknown and a process was not found for some pid.  the pid
    will be printed to stdout (not err) as when this flag is not provided.
2 - --timeout and timed out.  Implies that all processes were found
127 - other, typically argument parsing error.`)
		fmt.Fprintf(flag.CommandLine.Output(), "\n")
	}

	flag.Parse()

	if clkTck < 1 {
		fmt.Fprintln(os.Stderr, "clk-tck must be positive")
		flag.Usage()
		os.Exit(127)
	}

	if len(flag.Args()) < 1 {
		fmt.Fprintln(os.Stderr, "no pids provided")
		flag.Usage()
		os.Exit(127)
	}

	ctx := context.Background()
	if timeoutMs > 0 {
		var timeoutCancelFunc context.CancelFunc
		ctx, timeoutCancelFunc = context.WithTimeout(
			ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer timeoutCancelFunc()
	}

	type pidAndStarttime struct {
		pid int
		// 0 if absent.
		// this value is assumed to be what was read from /proc/<pid>/stat,
		// which defines its unit
		starttime uint64
	}

	pids := make([]pidAndStarttime, 0)
	for _, arg := range flag.Args() {
		pas := pidAndStarttime{}
		pidS, starttimeS, found := strings.Cut(arg, ":")
		pid, err := strconv.ParseUint(pidS, 10, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pid not a valid number: %v\n", arg)
			flag.Usage()
			os.Exit(127)
		}
		pas.pid = int(pid)

		if found {
			starttime, err := strconv.ParseUint(starttimeS, 10, 0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "starttime not a valid number: %v\n", arg)
				flag.Usage()
				os.Exit(127)
			}
			pas.starttime = starttime
		}

		pids = append(pids, pas)
	}

	pidfds := make([]*syscalls.Pidfd, len(pids))
	for i, pas := range pids {
		pidfd, err := syscalls.Open(pas.pid)
		if err == unix.ESRCH {
			exitUnknownProcess(pas.pid, errorOnUnknown)
		} else if err != nil {
			panic(err)
		}
		defer pidfd.Close()
		pidfds[i] = pidfd

		// determine if this process is the intended process by looking at its
		// starttime and comparing against the provided process starttime or the
		// global starttime read after starting all processes.
		// we do this _after_ opening the pidfd so that there is no possible
		// race where we determine it is the correct process, the process ends,
		// and another process starts with the same pid.
		isCorrectProcess, err := isCorrectProcess(pas.pid, pas.starttime, starttime, uint64(clkTck))
		if err != nil {
			panic(err)
		}
		if !isCorrectProcess {
			exitUnknownProcess(pas.pid, errorOnUnknown)
		}
	}

	// TODO: handle any retrying necessary, e.g., for signal wakeup.
	var pollTimeout time.Duration = -1 * time.Millisecond
	deadline, ok := ctx.Deadline()
	if ok {
		pollTimeout = time.Until(deadline)
		if pollTimeout < 0 {
			os.Exit(3)
		}
	}
	readyFds, err := syscalls.Poll(pidfds, int(pollTimeout.Milliseconds()))
	if err != nil {
		panic(err)
	}

	if len(readyFds) == 0 {
		// timed out
		os.Exit(2)
	}
	if len(readyFds) < 1 {
		panic("syscalls.Poll returned 0 ready file descriptors")
	}

	pid := readyFds[0].Pid
	fmt.Printf("%v\n", pid)
}

func isCorrectProcess(pid int, processStartTime uint64, globalStartTime uint64, clkTckHz uint64) (bool, error) {
	if processStartTime == 0 && globalStartTime == 0 {
		return true, nil
	}

	s, err := os.ReadFile(fmt.Sprintf("/proc/%v/stat", pid))
	if err != nil {
		// assume the process is gone
		// TODO: examine the errors more rigorously.  Examine if the
		// /proc/pid directory itself exists, implying that the process
		// is still running.
		return false, nil
	}

	statStartTime, err := readStatStarttime(string(s))
	if err != nil {
		return false, err
	}

	if processStartTime != 0 {
		return processStartTime == statStartTime, nil
	}

	// use globalStartTime
	// convert stat starttime to ns based on clkTckHz
	factor := 1_000_000_000 / clkTckHz
	startTime := statStartTime * factor
	return startTime < globalStartTime, nil
}

func exitUnknownProcess(pid int, errorOnUnknown bool) {
	fmt.Printf("%v\n", pid)
	retVal := 0
	if errorOnUnknown {
		retVal = 1
	}
	os.Exit(retVal)
}

func readStatStarttime(contents string) (uint64, error) {
	// get the starttime from the stat file
	// it is the 22nd field.  The 2nd field is the filename of the executable in
	// parenthesis.  It might have spaces and nested parenthesis and is the
	// only field that may be non-alphanumeric.  We will search for the
	// right-most ") " and assume the 3rd field starts immediately after.  Then
	// we find the 22nd overall field.

	lastIdx := strings.LastIndex(contents, ") ")
	if lastIdx == -1 {
		return 0, fmt.Errorf(
			"read proc stat starttime: \") \" not found (expected in field 2 of file).  Contents: %v",
			contents)
	}
	fieldsThreeAndUp := contents[lastIdx+2:]
	fields := strings.Split(fieldsThreeAndUp, " ")
	if len(fields) < 20 {
		return 0, fmt.Errorf(
			"read proc stat starttime: fewer fields than expected found after close parenthesis (assumed to be field 2).  Contents: %v",
			contents)
	}

	s := fields[19]
	statStartTime, err := strconv.ParseUint(s, 10, 0)
	if err != nil {
		return 0, fmt.Errorf(
			"read proc stat starttime: parsing starttime string.  Contents: %v: %w",
			contents,
			err)
	}
	return statStartTime, nil
}
