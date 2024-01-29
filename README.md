# waitn
Provides bash-like "wait -n" functionality as a separate command.

## Bash's wait

Bash provides `wait` options `-n` and `-p` to help with coordinating multiple subprocesses.
A brief primer on wait's behavior:
`wait` with no options or arguments waits until all child processes terminate and returns 0.
`wait <pid>...` waits until all listed pids terminate and returns the exit code of the last process listed.
`wait -n` waits until the next child process terminates and returns its error code
`wait -n <pid>...` waits until the next listed child process terminates and returns its error code
`-p VARNAME` may be added to `wait` or `wait -n` to assign to variable `VARNAME` the pid of the completed process.  This is useful for `wait -n` to determine which process finished, but also for all wait variants to help distinguish between a process completing or wait returning due to a trapped signal.  When wait returns due to a trapped signal the indicated variable will be the empty string.
`wait` will also returned on any trapped/handled signal, returning with exit code 128+signal number (e.g., 120 for SIGINT, 143 for SIGTERM).  Note that child processes may themselves return the same exit code, and so `-p` distinguishes between these cases.

One of the more confusing details of `wait -n` is that it internally manages a queue of completing processes.  Each call to `wait -n` will return a single completed process, even if that process completed prior to calling `wait -n`.  It's undocumented how the various options of `wait` impact this queue.  For example, calling `wait` with no options or arguments seems to clear all subprocess state -- you cannot then query for any exit code, even without `-n` (`wait <pid>`) will return 127 and complain that the process does not exist.  It's unclear if calls to `wait <pid>` remove the process from `wait -n`'s queue.

I find the internal management of this queue, without being able to inspect it directly, confusing.  This was the case when I discovered what I believe to be a bug in `wait -n` (a process terminated via signal prior to calling `wait -n` is never returned).

## Improvements
I can think of a few:
- a timeout option (`-t`): -1 to wait indefinitely (default), 0 to return immediately (nonblocking) -- useful for determining only if any of the named processes are still running, positive value to indicate in (likely) milliseconds how long to wait before returning.
- an "error on not-found" option (`-e`?): if any provided pid's processes could not be found or are found to already have been terminated then return immediately with an error while still writing the offending pid using `-p`.  The exit code for such processes can still be queried using `wait <pid>`.  This allows the caller to manage their own list of still-running processes by removing from the list any pid returned from previous calls to `wait -e` either because it terminated or could not be found.

## Build this as a separate command
I'd like to provide the above improvements by building `wait -n` or `wait -e` as a separate command.  I see numerous benefits to doing this:
- this fits with unix's "do one thing and do it well" philosophy.  We already see the addition of options and features to bash's `wait` causing some confusion (internally managed queue/state) and bugs.
- separation speeds development by decoupling tools and constraints.  I can build such a utility in any language I want instead of C; I don't need to convince the Bash maintainers to accept any change.
- a separate utility would be useful for other shells that don't support `wait -n` such as posix sh (ash, ksh) or zsh.

## waitn
Let's call such a command `waitn` as a reference to Bash's `wait -n`.  The intended behavior would be:
- it always behaves as `wait -n <pid>...`, never as just `wait`, `wait <pid>`, or `wait -n`.  The command will not be the parent of the processes we are waiting on and each invokation will be a distinct process; it will keep no state.
- `waitn` does not reap terminated processes or provide exit codes.  The parent process (your shell) must still call `wait <pid>` to do these things.  But the shell call to `wait <pid>` will never block if called after `waitn` returns indicating the process has ended.
- the chosen process's pid that has terminated or could not be found will be printed to stdout
- exit codes will indicate the result of `waitn`, _not the process returned_.  Again, to get the returned process's exit code you must call `wait <pid>`.
- the caller must manage a list/set of processes to watch, removing pids as they are returned.  Subsequent calls with the same pids will return the same pid, possibly indicating that the process does not exist.
- provide a timeout option with values -1 wait indefinitely (default), 0 return without blocking -- tells us if any of the processes could not be found, positive value to indicate maximum milliseconds to wait.

### Exit codes
- 0: a process was found and has terminated.  Its pid is printed to stdout.  This exit code implies that all processes were found.
- 1: a process was not found.  Its pid is printed to stdout.  It is possible that this process terminated prior to the call to `waitn`.
- 2: the timeout was reached before any of the provided processes terminated.  Note that returning with a timeout implies that all processes were found.
- 126: error parsing the input or processing options.  Printed to stderr
- 127: other error

### Implementation
on linux we will use https://man7.org/linux/man-pages/man2/pidfd_open.2.html
pidfds are file descriptors that are opened either with a call to `clone` or by pid with `pidfd_open`.  Their original purpose is to avoid unsafe of `signal` where a process terminates, its pid is reused, and the signal sent to the incorrect process of the same pid.  One can open a pidfd to a child process and if you haven't awaited that process you can guarantee that it refers to the correct process (even if it has terminated it is a zombie process since it hasn't been awaited).  From that point you may safely signal the process using the pidfd and it will never alias to another process.  Pidfds also allow polling/epolling the status of a process -- when the process terminates the fd is available for reading.

Note that we may still alias pids and accidentally wait on a process with a reused pid.  This would cause us to block longer than expected.  I'm not sure how to resolve this and identify whether we are waiting on the correct process.
Idea: start all processes, read a timestamp, query `jobs` to see which processes have already exited (don't `waitn` them, just get their exit codes from `wait`).  Pass the timestamp to `waitn` -- all processes must have started prior to that time.

### Example usage from Bash (should be similar for other shells)

#### Block and handle processes one at a time; use starttime to verify process identify

#### Start jobs with concurrency limit; use individual process starttimes

#### Forward SIGTERM; doesn't worry about pid reuse.

## Future
Take a look at libkqueue as a library to make this portable.  It doesn't look terribly well supported, but may simply be complete.
Consider whether Go is the right tool.  It's convenient and has remarkably easy access to syscalls for a high level language, but may produce large binaries, have quirks with system calls interacting with goroutines/threads, or otherwise not be accepted by the old school gnu/core utils community.  I suspect Rust, if anything, would be more appropriate, but this is typically the domain of C.