# waitn
Provides bash-like `wait -n` functionality as a separate command and with some semantic differences.

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
Otherwise you the utility may not find the process, and you may incorrectly
interpret it as having terminated.  It may also result in a panic.

return values:
0 - a process was found and completed; or a a process was not found and not
        --error-on-unknown.  The process presumably completed prior to this command
1 - --error-on-unknown and a process was not found for some pid.  the pid
    will be printed to stdout (not err) as when this flag is not provided.
2 - --timeout and timeout duration exceeded.  Implies that all processes were
        found
127 - other, typically argument parsing error.
```

## Building
If you have go installed
```
> go build ./cmd/waitn
```
If you don't want to install go but you do have Docker
```
> make
```

## Examples
See [examples](https://github.com/stevenpelley/waitn/tree/main/examples "examples")

## Implementation
Linux now provides [pidfd_open](https://man7.org/linux/man-pages/man2/pidfd_open.2.html).
pidfds are file descriptors that are opened with a call to `clone`, by pid with `pidfd_open`, or by opening the associated `/proc/<pid>` directory.  Their original purpose is to avoid unsafe signalling where a process terminates, its pid is reused, and the signal sent to the incorrect process of the same pid.  One can open a pidfd to a child process and if you haven't awaited that process you can guarantee that it refers to the correct process (even if it has terminated it is a zombie process since it hasn't been awaited).  From that point you may safely signal the process using the pidfd and it will never alias to another process.  Pidfds also allow polling/epolling the termination of a process -- when the process terminates the fd is available for reading.  More specifically, pidfds allow polling the termination of a _non-child_ process, which is what we rely on here.

Note that we may still alias pids and accidentally wait on a process with a reused pid.  This would cause us to block longer than expected.  In the future this can be addressed by locating a start timestamp for each process and passing it to wait, but this increases complexity substantially and this is a Linux-wide problem; I'm not going to solve it here.  If you're worried I recommend using the `-timeout` flag to periodically poll `jobs` and make sure some process didn't finish without you being made aware.

## Future
This is just a demonstration and proof of concept.  For widespread adoption consider:
- if go is the right tool.  Other languages may be more portable.
- if there are common libraries that can provide pidfd-like behavior across OSes.  libkqueue is a contender.
- there's not much testing here, and some syscall errors simply panic.  I didn't think much beyond this because it works for a demonstration.
- handling pid aliasing.  The way I see it this is a Unix-wide problem.  Pidfs provide a reliable means of referring to a process, but not of _naming_ a process.  We still need process names for commands (wait, kill) and to communicate about processes (logs, general human interaction involving processes).
- but... something might also be done by looking at process starttime in /proc/<pid>/stat field 22.  I _think_ this is a time, in CLK_TCK, corresponding to clock_gettime CLOCK_BOOTTIME.  One could start a number of processes, read from CLOCK_BOOTTIME (possibly verify that all process starttimes were before the read time), and pass this time to waitn.  Waitn would open pidfds, read the starttimes of all processes, and for any with starttime after the read time conclude it must be a new process.  If mixing process start times you could also pass the starttime along with pids.  I started down this road, put some code in internal/proc/stat.go, and backed out; it's cumbersome enough that I don't think people will use it.  I'm also not terribly confident in the approach (what clock is read to set the process starttime?  It's done in the kernel and is difficult to trace)
