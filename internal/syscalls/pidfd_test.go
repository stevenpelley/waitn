package syscalls

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPidfd(t *testing.T) {
	require := require.New(t)

	cmd, procChan := createTestProcess(require)
	pidFile := PidFile{Pid: cmd.Process.Pid}
	err := pidFile.Start()
	if err != nil {
		pidFile.Close()
		t.Fatal(err)
	}

	pidfdChan := make(chan error)
	go func() {
		defer pidFile.Close()
		err := pidFile.BlockUntilDoneOrClosed()
		pidfdChan <- err
		close(pidfdChan)
	}()

	time.Sleep(1 * time.Second)

	// neither should have yet completed
	select {
	case <-procChan:
		t.Fatal("process already finished")
	case <-pidfdChan:
		t.Fatal("pidfd already finished")
	default:
	}

	//cancelFunc()
	procResult := <-procChan
	require.NoError(procResult.err, procResult.msg)

	err = <-pidfdChan
	require.NoError(err)
}

type processResult struct {
	msg string
	err error
}

// starts a test process (sleep) and returns the command (while provides the
// pid), a channel providing output and any execution error, and an error from
// starting the process.  If this provides an error then the process did not
// start and other outputs will be nil
func createTestProcess(require *require.Assertions) (
	*exec.Cmd, <-chan processResult) {
	c := make(chan processResult)
	cmd := exec.Command("sleep", "2")
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Start()
	require.NoError(err)

	go func() {
		err := cmd.Wait()
		s := out.String()
		c <- processResult{msg: s, err: err}
		close(c)
	}()
	return cmd, c
}
