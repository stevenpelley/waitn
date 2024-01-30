package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/stevenpelley/waitn/internal/syscalls"
	"golang.org/x/sys/unix"
)

// exit codes
const (
	PROCESS_TERMINATED        = 0
	PROCESS_NOT_FOUND_SUCCESS = 0
	PROCESS_NOT_FOUND_ERROR   = 1
	TIMEOUT_ERROR             = 2
	INPUT_ERROR               = 127
)

// CLI flags
var (
	errorOnUnknown bool
	timeoutMs      int64
)

func main() {
	// parses using flag
	ctx := prepare()

	pids := make([]int, 0)
	for _, arg := range flag.Args() {
		pid, err := strconv.Atoi(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pid not a valid number: %v\n", arg)
			flag.Usage()
			os.Exit(INPUT_ERROR)
		}
		pids = append(pids, pid)
	}

	pidfds := make([]*syscalls.Pidfd, len(pids))
	for i, pid := range pids {
		pidfd, err := syscalls.Open(pid)
		if err == unix.ESRCH {
			exitUnknownProcess(pid, errorOnUnknown)
		} else if err != nil {
			panic(err)
		}
		defer pidfd.Close()
		pidfds[i] = pidfd
	}

	// TODO: handle any retrying necessary, e.g., for signal wakeup.
	var pollTimeout time.Duration = -1 * time.Millisecond
	deadline, ok := ctx.Deadline()
	if ok {
		pollTimeout = time.Until(deadline)
		if pollTimeout < 0 {
			os.Exit(TIMEOUT_ERROR)
		}
	}

	readyFds, err := syscalls.Poll(pidfds, int(pollTimeout.Milliseconds()))
	if err != nil {
		panic(err)
	}

	if len(readyFds) == 0 {
		// timed out
		os.Exit(TIMEOUT_ERROR)
	}

	pid := readyFds[0].Pid
	fmt.Printf("%v\n", pid)
}

func prepare() context.Context {
	errorOnUnknownUsage := "if any process cannot be found return an error code, not 0"
	flag.BoolVar(&errorOnUnknown, "error-on-unknown", false, errorOnUnknownUsage)
	flag.BoolVar(&errorOnUnknown, "u", false, errorOnUnknownUsage)

	timeoutUsage := "timeout in ms.  Negative implies no timeout.  Zero means to return immediately if no process is ready"
	flag.Int64Var(&timeoutMs, "timeout", 0, timeoutUsage)
	flag.Int64Var(&timeoutMs, "t", 0, timeoutUsage)

	flag.Usage = func() {
		fmt.Fprintln(
			flag.CommandLine.Output(),
			`wait for the first of several processes to terminate, as in Bash's wait -n.
Usage: waitn [-u] [-t <timeout>] <pid>...`)
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output())
		fmt.Fprint(
			flag.CommandLine.Output(),
			`Behavior when no process can be found for a pid is deterministic.  The first
pid to be not found is returned.  Only then the first process to complete is
returned.  Subsequent calls with the same list of pids should return the same
pid or some pid listed earlier (assuming no pid reuse)

NOTE WELL: pids may be reused; processes may alias.  If this happens a call to
waitn may block for the incorrect process with the same pid.

NOTE WELL: this utility uses Linux's pidfd to wait for non-child processes.  It
is up to you to ensure that the processes you wait for are visible to this call.
Otherwise you the utility may not find the process, and you may incorrectly
interpret it as having terminated.  It may also result in a panic.

return values:
0 - a process was found and completed; or a a process was not found and not
	--error-on-unknown.  The process presumably completed prior to this command
1 - --error-on-unknown and a process was not found for some pid.  the pid
    will be printed to stdout (not err) as when this flag is not provided.
2 - --timeout and timeout duration exceeded.  Implies that all processes were
	found
127 - other, typically argument parsing error.`)
		fmt.Fprintln(flag.CommandLine.Output())
	}

	flag.Parse()

	if len(flag.Args()) < 1 {
		fmt.Fprintln(os.Stderr, "no pids provided")
		flag.Usage()
		os.Exit(INPUT_ERROR)
	}

	ctx := context.Background()
	if timeoutMs > 0 {
		var timeoutCancelFunc context.CancelFunc
		ctx, timeoutCancelFunc = context.WithTimeout(
			ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer timeoutCancelFunc()
	}
	return ctx
}

func exitUnknownProcess(pid int, errorOnUnknown bool) {
	fmt.Printf("%v\n", pid)
	retVal := PROCESS_NOT_FOUND_SUCCESS
	if errorOnUnknown {
		retVal = PROCESS_NOT_FOUND_ERROR
	}
	os.Exit(retVal)
}
