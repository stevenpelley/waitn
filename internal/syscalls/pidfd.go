package syscalls

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// PidFile must be be started before blocking.
// it must be closed after finished blocking or whenever finished using.
type PidFile struct {
	Pid  int
	file *os.File
	conn syscall.RawConn
}

// setup/start the process of waiting for the process.
// PidFiles run as a 2-stop start/block so the caller immediately knows if there
// is an error creating the file.
// If no process is found with the provided pid the returned error will satisfy
// errors.Is(err, unix.ESRCH)
// a PidFile must be started exactly once.
func (pf *PidFile) Start() error {
	if pf.file != nil {
		panic("PidFile already started")
	}

	fd, err := unix.PidfdOpen(pf.Pid, unix.PIDFD_NONBLOCK)
	if err != nil {
		return err
	}
	pf.file = os.NewFile(uintptr(fd), fmt.Sprintf("pidfd:%v", pf.Pid))

	conn, err := pf.file.SyscallConn()
	if err != nil {
		err2 := pf.file.Close()
		err = errors.Join(err, err2)
		return err
	}
	pf.conn = conn

	return nil
}

// block until the process completes. Returns any error from reading the
// pidfile.  This does _not_ wait/reap the process, nor does it return any exit
// code or error associated with the process's execution.
func (pf *PidFile) BlockUntilDoneOrClosed() error {
	if pf.file == nil {
		panic("PidFile not started")
	}
	callCount := 0
	return pf.conn.Read(func(fd uintptr) (done bool) {
		// "read" twice (don't ever actually read, pidfd doesn't support)
		// the first read is not done and so it will poll.
		// once the poll completes it will read again and we will say done
		callCount += 1
		return callCount > 1
	})
}

// close the pidfile.
func (pf *PidFile) Close() error {
	return pf.file.Close()
}
