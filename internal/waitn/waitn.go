package waitn

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/stevenpelley/waitn/internal/syscalls"
	"golang.org/x/sys/unix"
)

// exit codes
const (
	PROCESS_TERMINATED      = 0
	PROCESS_NOT_FOUND_ERROR = 1
	TIMEOUT_ERROR           = 2
	INPUT_ERROR             = 127
)

// error strings
const (
	PID_PARSE_ERR = "pid is not a valid number"
)

type ExitError struct {
	Message      string
	ExitCode     int
	DisplayUsage bool
	Cause        error
}

func (err *ExitError) Error() string {
	if err.Cause == nil {
		return err.Message
	}
	return fmt.Sprintf("%v: %v", err.Message, err.Cause)
}

func (err *ExitError) Unwrap() error {
	return err.Cause
}

var ProcessNotFoundErr *ExitError = &ExitError{
	Message:      "",
	ExitCode:     PROCESS_NOT_FOUND_ERROR,
	DisplayUsage: false,
	Cause:        nil}

var TimeoutErr *ExitError = &ExitError{
	Message:      "timed out",
	ExitCode:     TIMEOUT_ERROR,
	DisplayUsage: false,
	Cause:        nil}

// cannot be 0, so 0 means no result
type ResultPid int

// Set up all the pid files, or determine that we are done.
// Returns at least one of:
// list of pid files -- continue to poll the pid files if not nil
// resultPid -- failed to find a process, treat it as completed.  0 is none
// exitError -- an error that will end the program.  May contain an *exitError
//
// may not return a non-nil list of pid files alongside a non-zero resultPid or
// error.
func SetupPidFiles(args []string, errorOnUnknown bool) ([]*syscalls.PidFile, ResultPid, error) {
	pids := make([]int, len(args))
	for i, arg := range args {
		var pid int
		pid, err := strconv.Atoi(arg)
		if err != nil {
			exitErr := &ExitError{
				Message:      PID_PARSE_ERR,
				ExitCode:     INPUT_ERROR,
				DisplayUsage: true,
				Cause:        err}
			return nil, 0, exitErr
		}
		pids[i] = pid
	}

	pidFiles := make([]*syscalls.PidFile, len(pids))
	doDefer := true
	defer func() {
		if !doDefer {
			return
		}
		for _, pidFile := range pidFiles {
			if pidFile == nil {
				continue
			}
			if err := pidFile.Close(); err != nil {
				panic(err)
			}
		}
	}()
	for i, pid := range pids {
		pidFile := &syscalls.PidFile{Pid: pid}
		err := pidFile.Start()
		if errors.Is(err, unix.ESRCH) {
			retPid := ResultPid(pid)
			exitErr := exitErrorUnknownProcess(errorOnUnknown)
			return nil, retPid, exitErr
		} else if err != nil {
			panic(err)
		}
		pidFiles[i] = pidFile
	}

	// We set up all pid files without error.  Disable the deferred close.
	// Caller takes responsibility for closing the files.
	doDefer = false

	// pidFiles is set, retPid and err are 0/nil
	return pidFiles, 0, nil
}

func exitErrorUnknownProcess(errorOnUnknown bool) error {
	if !errorOnUnknown {
		return nil
	}
	// this error simply changes the exit code while still providing a result
	return ProcessNotFoundErr
}

// wait for the first pid file to finish or for the context to end.  Close all
// resources and return either a non-zero resultPid or non-nil exitError
func WaitForPidFile(ctx context.Context, pidFiles []*syscalls.PidFile) (
	ResultPid, error) {
	if pidFiles == nil {
		panic("WaitForPidFile: pidFiles is nil")
	}

	// close files to unblock all waiting goroutines.  We'll do this after
	// receiving a pid or on a timeout.  We also defer this so that we'll
	// unblock those goroutines on panic.
	closePidFilesOnce := sync.OnceFunc(func() {
		for _, pidFile := range pidFiles {
			if err := pidFile.Close(); err != nil {
				panic(err)
			}
		}
	})
	defer closePidFilesOnce()

	// channel is buffered size 1.  pidfile goroutines attempt to write
	// nonblocking.  Guaranteed to write the first result.  If anyone else
	// managed to write it will be ignored.
	// on error we'll panic from the main goroutine, which should make it easier
	// to handle gracefully in the future should we choose to.
	type pidFileResult struct {
		pidFile *syscalls.PidFile
		err     error
	}
	c := make(chan pidFileResult, 1)

	// setup a goroutine for each pidfile, to be joined on result or timeout.
	wg := sync.WaitGroup{}
	wg.Add(len(pidFiles))
	for _, pidFile := range pidFiles {
		pidFile := pidFile
		go func() {
			err := pidFile.BlockUntilDoneOrClosed()
			select {
			case c <- pidFileResult{pidFile: pidFile, err: err}:
			default:
			}
			wg.Done()
		}()
	}

	// wait for the first process to finish or a timeout
	var pid ResultPid
	var exErr error
	select {
	case result := <-c:
		pid = ResultPid(result.pidFile.Pid)
		if result.err != nil {
			panic(fmt.Sprintf("error on PidFile %v: %v", pid, result.err))
		}
	case <-ctx.Done():
		exErr = TimeoutErr
	}

	// unblock all pidfile goroutines and join them.
	closePidFilesOnce()
	wg.Wait()
	return pid, exErr
}
