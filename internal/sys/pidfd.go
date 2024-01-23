package sys

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Not thread safe
type Pidfd struct {
	fd  int
	Pid int
	// close() should not be retried on error
	hasClosed bool
}

func Open(pid int) (*Pidfd, error) {
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return nil, err
	}
	return &Pidfd{fd: fd, Pid: pid}, nil
}

func (fd *Pidfd) Close() error {
	if !fd.hasClosed {
		return unix.Close(fd.fd)
	}
	return nil
}

// returns a slice of the input fds that are ready for reading, and an error.
func Poll(fds []*Pidfd, timeoutMs int) ([]*Pidfd, error) {
	pollFds := make([]unix.PollFd, len(fds))
	for i := range fds {
		pollFds[i].Fd = int32(fds[i].fd)
		pollFds[i].Events = unix.POLLIN
	}
	_, err := unix.Poll(pollFds, timeoutMs)
	if err != nil {
		return nil, err
	}
	readyFds := make([]*Pidfd, 0)
	for i, pollFd := range pollFds {
		if pollFd.Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return nil, fmt.Errorf("error polling fd for pid %v. Revents: %v", fds[i].fd, pollFd.Revents)
		}

		if pollFd.Revents&unix.POLLIN != 0 {
			readyFds = append(readyFds, fds[i])
		}
	}

	return readyFds, err
}
