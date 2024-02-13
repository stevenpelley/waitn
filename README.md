# waitn
Provides bash-like `wait -n` functionality as a separate command and with some semantic differences.
This project currently supports only Linux by relying on pidfds.

See [my project page](https://stevenpelley.github.io/waitn/article) for an article I wrote about building this project.

## Usage
```
wait for the first of several processes to terminate, as in Bash's wait -n.
Usage: waitn [-u] [-t <timeout>] <pid>...
  -error-on-unknown
        if any process cannot be found return an error code, not 0
  -t int
        shorthand for -timeout
  -timeout int
        timeout in ms.  Negative implies no timeout.  Zero means to return immediately if no process is ready
  -u    shorthand for -error-on-unknown

Behavior when no process can be found for a pid is deterministic.  The first
pid to be not found is returned.  Only then the first process to complete is
returned.  Subsequent calls with the same list of pids should return the same
pid or some pid listed earlier (assuming no pid reuse)

NOTE WELL: pids may be reused; processes may alias.  If this happens a call to
waitn may block for the incorrect process with the same pid.

NOTE WELL: this utility uses Linux's pidfd to wait for non-child processes.  It
is up to you to ensure that the processes you wait for are visible to this call.
Otherwise the utility may not find the process, and you may incorrectly
interpret it as having terminated.  It may also result in a panic.

return values:
0 - a process was found and completed; or a a process was not found and not
        -error-on-unknown.  The process presumably completed prior to this command
1 - -error-on-unknown and a process was not found for some pid.  the pid
    will be printed to stdout (not err) as when this flag is not provided.
2 - -timeout and timeout duration exceeded.  Implies that all processes were
        found
127 - other, typically argument parsing error.
```

## Use Cases
This is intended to be a near drop-in replacement for bash's `wait -n` that
additionally returns a pid for a process that _previously_ completed.  You must
still `wait <pid>` to get the exit code as only the parent process can (syscall)
wait.  See `examples/common.sh` for a script to wrap this functionality and
additionally return immediately on any trapped signal, as shell `wait` does.

This can then be used with posix shells and zsh, which have no `wait -n`
equivalent.

This can also be used to block on a _parent_ process terminating, in cases where
you want subprocesses to terminate and you don't want to coordinate a SIGHUP.

## Building and Development
If you have Go installed
```
> go build ./cmd/waitn
```
If you don't want to install Go but you do have Docker
```
> make
```

This project uses a [devcontainer](https://containers.dev/).
See [VSCode devcontainers](https://code.visualstudio.com/docs/devcontainers/containers) or your IDE's documentation.
It "works for me" in this setup with Linux 5.3+.

## Examples
See [examples](https://github.com/stevenpelley/waitn/tree/main/examples "examples")

## Implementation
Linux now provides [pidfd_open](https://man7.org/linux/man-pages/man2/pidfd_open.2.html).
pidfds are file descriptors that are opened with a call to `clone`, by pid with `pidfd_open`, or by opening the associated `/proc/<pid>` directory.  Their original purpose was to avoid unsafe signalling where a process terminates, its pid is reused, and the signal sent to the incorrect process of the same pid.  One can open a pidfd to a child process and if you haven't awaited that process you can guarantee that it refers to the correct process (even if it has terminated it is a zombie process since it hasn't been awaited).  From that point you may safely signal the process using the pidfd and it will never alias to another process.  Pidfds also allow polling/epolling the termination of a process -- when the process terminates the fd is available for reading.  More specifically, pidfds allow polling the termination of a _non-child_ process, which is what we rely on here.

Note that we may still alias pids and accidentally wait on a process with a reused pid.  This would cause us to block longer than expected.  In the future this can be addressed by locating a start timestamp for each process and passing it to wait, but this increases complexity substantially and this is a Linux-wide problem; I'm not going to solve it here.  If you're worried I recommend using the `-timeout` flag to periodically poll `jobs` and make sure some process didn't finish without you being made aware.

## Future
This is just a demonstration and proof of concept.  For widespread adoption consider:
- if go is the right tool.  Other languages may be more portable.  I'm impressed by Go's ability to call syscalls and then integrate a "non-standard" (i.e., can be epolled for readability but can't call read()) file descriptor into os.File.
- portability: bsd provides kqueue with filter EVFILT_PROC accepting a PID. Windows has OpenProcessToken and WaitForMultipleObjects.
- if there are common libraries that can provide pidfd-like behavior across OSes.  libkqueue is a contender.
- any unexpected error panics.  This could be more graceful but at the moment I want to force the caller to see the stack trace and deal with it.
- handling pid aliasing.  The way I see it this is a Unix-wide problem.  Pidfs provide a reliable means of referring to a process, but not of _naming_ a process.  We still need process names for commands (wait, kill) and to communicate about processes (logs, general human interaction involving processes).
- but... something might also be done by looking at process starttime in /proc/<pid>/stat field 22.  I _think_ this is a time, in CLK_TCK, corresponding to clock_gettime CLOCK_BOOTTIME.  One could start a number of processes, read from CLOCK_BOOTTIME (possibly verify that all process starttimes were before the read time), and pass this time to waitn.  Waitn would open pidfds, read the starttimes of all processes, and for any with starttime after the read time conclude it must be a new process.  If mixing process start times you could also pass the starttime along with pids.  I started down this road, put some code in internal/proc/stat.go, and backed out; it's cumbersome enough that I don't think people will use it.  I'm also not terribly confident in the approach (what clock is read to set the process starttime?  It's done in the kernel and is difficult to trace)
- this could also be addressed if various tools get comfortable with duplicating/transferring file descriptors of pidfds via unix domain sockets or pidfd_getpidfd.  This is some fringe stuff.  Imagine a bash builtin that told you the file descriptor for a subprocess, or a builtin variable telling you this file descriptor as $? returns the pid of the last asynchronous command.  Then you could duplicate this descriptor.  You'd have to indicate to bash that you want to pin that fd so it isn't reused (sigh, everything is just a number that can be reused)
