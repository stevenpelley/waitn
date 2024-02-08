package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/stevenpelley/waitn/internal/waitn"
)

// CLI flags
type cliFlags struct {
	errorOnUnknown bool
	timeoutMs      int64
}

// returns a context for waiting/timeout, a function to cancel that context
// (should be deferred), and flags from the CLI
func prepare() (context.Context, context.CancelFunc, cliFlags) {
	cliFlags := cliFlags{}

	errorOnUnknownUsage := "if any process cannot be found return an error code, not 0"
	flag.BoolVar(&cliFlags.errorOnUnknown, "error-on-unknown", false, errorOnUnknownUsage)
	flag.BoolVar(&cliFlags.errorOnUnknown, "u", false, "shorthand for -error-on-unknown")

	timeoutUsage := "timeout in ms.  Negative implies no timeout.  Zero means to return immediately if no process is ready"
	flag.Int64Var(&cliFlags.timeoutMs, "timeout", 0, timeoutUsage)
	flag.Int64Var(&cliFlags.timeoutMs, "t", 0, "shorthand for -timeout")

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
		os.Exit(waitn.INPUT_ERROR)
	}

	ctx := context.Background()
	var contextCancel context.CancelFunc = func() {}
	if cliFlags.timeoutMs > 0 {
		ctx, contextCancel = context.WithTimeout(
			ctx, time.Duration(cliFlags.timeoutMs)*time.Millisecond)
	}
	return ctx, contextCancel, cliFlags
}

func exitIfResultOrError(pid waitn.ResultPid, e error) {
	if pid != 0 {
		fmt.Printf("%v\n", pid)
	}
	if e != nil {
		err, ok := e.(*waitn.ExitError)
		if !ok {
			panic(fmt.Sprintf("error is not of type *waitn.ExitError: %v", e))
		}
		msg := err.Error()
		if len(msg) > 0 {
			fmt.Fprintln(os.Stderr, msg)
		}
		if err.DisplayUsage {
			flag.Usage()
		}
		os.Exit(err.ExitCode)
	}
	if pid != 0 {
		os.Exit(waitn.PROCESS_TERMINATED)
	}
}

func main() {
	// parses using flag
	ctx, ctxCancel, cliFlags := prepare()
	defer ctxCancel()

	pidFiles, retPid, exitErr := waitn.SetupPidFiles(
		flag.Args(), cliFlags.errorOnUnknown)
	exitIfResultOrError(retPid, exitErr)

	retPid, exitErr = waitn.WaitForPidFile(ctx, pidFiles)
	exitIfResultOrError(retPid, exitErr)

	panic("no result or error at end of main")
}
