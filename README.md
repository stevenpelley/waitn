# waitn
Provides bash-like "wait -n" functionality as a separate command and with some semantic differences.

## Bash's wait

Bash provides `wait` options `-n` and `-p` to help with coordinating multiple subprocesses.
A brief primer on wait's behavior:
`wait` with no options or arguments waits until all child processes terminate and returns 0.
`wait <pid>...` waits until all listed pids terminate and returns the exit code of the last process listed.
`wait -n` waits until the next child process terminates and returns its error code
`wait -n <pid>...` waits until the next listed child process terminates and returns its error code
`-p VARNAME` added to `wait` or `wait -n` assigns to variable `VARNAME` the pid of the completed process.  This is useful for `wait -n` to determine which process finished, but also for all wait variants to help distinguish between a process completing or wait returning due to a trapped signal.  When wait returns due to a trapped signal the indicated variable will be the empty string.
`wait` will also returned on any trapped/handled signal, returning with exit code 128+signal number (e.g., 120 for SIGINT, 143 for SIGTERM).  Note that child processes may themselves return the same exit code, and so `-p` distinguishes between these cases.

One of the complications of `wait -n` is that it only returns jobs that finish after it is called.  There may be races where jobs finish prior to the first call to `wait -n`, or finish between calls to `wait -n`.  Such jobs will not be returned.  (There is a wide misunderstanding and at least one bug, see https://lists.gnu.org/archive/html/bug-bash/2024-01/msg00137.html).

## Improvements
Some other improvements may help:
- a timeout option (`-t`): -1 to wait indefinitely (default), 0 to return immediately (nonblocking) -- useful for determining only if any of the named processes are still running, positive value to indicate in (likely) milliseconds how long to wait before returning.
- an "error on not-found" option (`-e`?): if any provided pid's processes could not be found or are found to already have been terminated then return immediately with an error while still writing the offending pid using `-p`.  The exit code for such processes can still be queried using `wait <pid>`.  This allows the caller to manage their own list of still-running processes by removing from the list any pid returned from previous calls to `wait -e` either because it terminated or could not be found.

## Build this as a separate command
I'd like to provide the above improvements by building `wait -n` as a separate command.  I see numerous benefits to doing this:
- this fits with unix's "do one thing and do it well" philosophy.  We already see the addition of options and features to bash's `wait` causing some confusion (internally managed queue/state) and bugs.
- separation speeds development by decoupling tools and constraints.  I can build such a utility in any language I want instead of C; I don't need to convince the Bash maintainers to accept any change.
- a separate utility would be useful for other shells that don't support `wait -n` such as posix sh (ash, ksh) or zsh.

## waitn
```
wait for the first of several processes to terminate, as in Bash's wait -n.
Usage: waitn [-u] [-t <timeout>] <pid>...
  -error-on-unknown
        if any process cannot be found return an error code, not 0
  -t int
        timeout in ms.  Negative implies no timeout.  Zero means to return immediately if no process is ready
  -timeout int
        timeout in ms.  Negative implies no timeout.  Zero means to return immediately if no process is ready
  -u    if any process cannot be found return an error code, not 0

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

### Implementation
Linux now provides https://man7.org/linux/man-pages/man2/pidfd_open.2.html
pidfds are file descriptors that are opened either with a call to `clone` or by pid with `pidfd_open`.  Their original purpose is to avoid unsafe of `signal` where a process terminates, its pid is reused, and the signal sent to the incorrect process of the same pid.  One can open a pidfd to a child process and if you haven't awaited that process you can guarantee that it refers to the correct process (even if it has terminated it is a zombie process since it hasn't been awaited).  From that point you may safely signal the process using the pidfd and it will never alias to another process.  Pidfds also allow polling/epolling the status of a process -- when the process terminates the fd is available for reading.

Note that we may still alias pids and accidentally wait on a process with a reused pid.  This would cause us to block longer than expected.  In the future this can be addressed by locating a start timestamp for each process and passing it to wait, but this increases complexity substantially and this is a Linux-wide problem; I'm not going to solve it here.

### Example usage from Bash (should be similar for other shells)

#### Block and handle processes one at a time

#### Start jobs with concurrency limit

#### Forward SIGTERM

## Future
Take a look at libkqueue as a library to make this portable.  It doesn't look terribly well supported, but may simply be complete.
I could also call bsd's libc from cgo directly.  Windows appears to have the needed syscalls as well.
Consider whether Go is the right tool.  It's convenient and has remarkably easy access to syscalls for a high level language, but may produce large binaries, have quirks with system calls interacting with goroutines/threads, or otherwise not be accepted by the old school gnu/core utils community.  I suspect Rust, if anything, would be more appropriate, but this is typically the domain of C.