package waitn

import (
	"context"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSetupPidFiles(t *testing.T) {
	require := require.New(t)

	// error parsing
	{
		pidFiles, retPid, err := SetupPidFiles(
			[]string{"asdf"}, false)
		require.Nil(pidFiles)
		require.EqualValues(0, retPid)
		var exitErr *ExitError
		require.ErrorAs(err, &exitErr)
		require.Equal(INPUT_ERROR, exitErr.ExitCode)
		require.Equal(PID_PARSE_ERR, exitErr.Message)
		require.ErrorIs(exitErr, strconv.ErrSyntax)
	}

	// pid not found, success

	// find a pid with no process
	var pid int
	var pidStr string
	{
		bytes, err := os.ReadFile("/proc/sys/kernel/pid_max")
		require.NoError(err)
		maxPid, err := strconv.Atoi(strings.TrimSpace(string(bytes)))
		require.NoError(err)
		for {
			pid = rand.Intn(maxPid - 1)
			pid += 1
			proc, err := os.FindProcess(pid)
			// always returns a process for unix
			require.NoError(err)
			err = proc.Signal(syscall.Signal(0))
			if err != nil {
				// no process exists
				break
			}
		}
		pidStr = strconv.Itoa(pid)

		pidFiles, retPid, err := SetupPidFiles(
			[]string{pidStr}, false)
		require.Nil(pidFiles)
		require.EqualValues(pid, retPid)
		require.NoError(err)
	}

	// pid not found, error
	{
		pidFiles, retPid, err := SetupPidFiles(
			[]string{pidStr}, true)
		require.Nil(pidFiles)
		require.EqualValues(pid, retPid)
		require.ErrorIs(err, ProcessNotFoundErr)
	}

	// pid found
	{
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		cmd, err := createTestSleep(ctx, "10")
		require.NoError(err)
		pid := cmd.Process.Pid

		pidFiles, retPid, err := SetupPidFiles(
			[]string{strconv.Itoa(pid)}, true)
		require.NoError(err)
		require.EqualValues(0, retPid)
		require.Len(pidFiles, 1)
		pidFile := pidFiles[0]
		require.Equal(pid, pidFile.Pid)
		cancelFunc()
		cmd.Wait()
	}
}

func TestWaitForPidFile(t *testing.T) {
	require := require.New(t)

	// timeout
	{
		procCtx, cancelProc := context.WithCancel(context.Background())
		defer cancelProc()
		cmd, err := createTestSleep(procCtx, "10")
		require.NoError(err)
		pid := cmd.Process.Pid

		pidFiles, retPid, err := SetupPidFiles(
			[]string{strconv.Itoa(pid)}, true)
		require.NoError(err)
		require.EqualValues(0, retPid)
		require.Len(pidFiles, 1)

		waitCtx, cancelTimeout := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancelTimeout()
		retPid, err = WaitForPidFile(waitCtx, pidFiles)
		require.ErrorIs(err, TimeoutErr)
		require.EqualValues(0, retPid)
		cancelProc()
	}

	// single pid, completes
	{
		var err error
		procCtx := context.Background()
		cmd, err := createTestSleep(procCtx, "0.1")
		require.NoError(err)
		pid := cmd.Process.Pid

		pidFiles, retPid, err := SetupPidFiles(
			[]string{strconv.Itoa(pid)}, true)
		require.NoError(err)
		require.EqualValues(0, retPid)
		require.Len(pidFiles, 1)

		waitCtx, cancelTimeout := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelTimeout()
		retPid, err = WaitForPidFile(waitCtx, pidFiles)
		require.NoError(err)
		require.EqualValues(cmd.Process.Pid, retPid)
	}

	// multiple pid, completes
	{
		var err error
		procCtx := context.Background()
		cmd1, err := createTestSleep(procCtx, "0.1")
		require.NoError(err)
		pid1 := cmd1.Process.Pid
		cmd2, err := createTestSleep(procCtx, "2.0")
		require.NoError(err)
		pid2 := cmd2.Process.Pid

		pidFiles, retPid, err := SetupPidFiles(
			[]string{strconv.Itoa(pid1), strconv.Itoa(pid2)}, true)
		require.NoError(err)
		require.EqualValues(0, retPid)
		require.Len(pidFiles, 2)

		waitCtx, cancelTimeout := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelTimeout()
		retPid, err = WaitForPidFile(waitCtx, pidFiles)
		require.NoError(err)
		require.EqualValues(cmd1.Process.Pid, retPid)

		// cmd2 should still be running
		err = cmd2.Process.Signal(syscall.Signal(0))
		require.NoError(err)
	}
}

// need to set duration
// need to be able to cancel
func createTestSleep(ctx context.Context, sleepDuration string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "sleep", sleepDuration)
	err := cmd.Start()
	return cmd, err
}
