# waitn
Provides bash-like `wait -n` functionality as a separate command and with some semantic differences.  See the [Project Page](https://github.com/stevenpelley/waitn) for usage and source.

## What Problem Does `wait` Solve?
`wait` is a POSIX shell command allowing you to wait for a specific subprocess to exit, returning its exit code.  It can also be used to wait for numerous subprocesses to finish (or all bg jobs, if no arguments given).

But let's say you want to know as soon as _any_ job finishes.  POSIX `wait` can't do this.  The only shell I'm aware of that provides a mechanism to do this is Bash, with `wait -n`.  `wait -n` returns the next job that finishes, returning its exit code and allowing you to assign its pid to a variable (`-p`).  See `man bash` or if running bash, `help wait`.  This allows you to build scripts that manage simple task queues or manage interacting subprocesses, all while handling signals and responding to subprocesses terminating.  Shell is the simplest way to _start_ subprocesses, so it's be convenient if it also allows you to easily _manager and coordinate_ those subprocesses.

An example: let's say you are building a service that runs inside a Docker container.  You want your container to have multiple processes, for example for legacy constraints or to run Java Flight Recorder or `perf` inside the container.  So you need to:
1. start all the processes.  The first process is your "main" process.  Others, such as `perf`, need the pid of your main process.  There are dependencies and interaction between processes.
2. redirect and pipe input and output.  This may be simply to files as logs or for later data processing.
3. monitor the processes.  If the main process ends then SIGTERM the others and wait for them to terminate as well.  If other processes end then you may choose to restart it, end all processes and stop the container, or simply continue.
4. handle SIGTERM, propagate it to subprocesses by signalling them in turn, and then wait for them to terminate (as in 3.)

Shell handles the first 2 requirements easily, but without more flexible tooling such as `wait -n` can't handler the latter 2 requirements.  Other languages (in my opinion) may handle subprocess termination and signals better but become cumbersome when creating process pipelines.  For example, Python's `subprocess.run` (https://docs.python.org/3/library/subprocess.html) has 14 keyword arguments and additionally accepts other `Popen` keyword arguments.  In many cases Python `subprocess` is convenient, but it can be overwhelming at first.  Similarly, Python signal handling (https://docs.python.org/3/library/signal.html) is both confusing and error-prone.  Handlers always run on the initial Python thread and run on a stack frame created out of thin air (and so raising an Exception is generally not safe).  I haven't found any good resources describing what happens if Python is in a blocking syscall when the handler runs.  There's perhaps no perfect solution, but shell can be the simplest with the right commands.

## Bash's `wait -n`

Bash provides `wait` options `-n` and `-p` to help with coordinating multiple subprocesses.
A brief primer on wait's behavior:
- `wait` with no options or arguments waits until all child processes terminate and returns 0.
- `wait <pid>...` waits until all listed pids terminate and returns the exit code of the last process listed.
- `wait -n` waits until the next child process terminates and returns its error code.
- `wait -n <pid>...` waits until the next listed child process terminates and returns its error code.
- `-p VARNAME` added to `wait` or `wait -n` assigns to variable `VARNAME` the pid of the completed process.  This is useful for `wait -n` to determine which process finished, but also for all wait variants to help distinguish between a process completing or wait returning due to a trapped signal.  When wait returns due to a trapped signal the indicated variable will be the empty string.
`wait` will also returned on any trapped/handled signal, returning with exit code 128+signal number (e.g., 120 for SIGINT, 143 for SIGTERM).  Note that child processes may themselves return the same exit code, and so `-p` distinguishes between these cases.

A complication of `wait -n` is that it only returns jobs that finish after it is called.  There may be races where jobs finish prior to the first call to `wait -n`, or finish between calls to `wait -n`.  Such jobs will not be returned.  (There is a wide misunderstanding and at least one bug, see https://lists.gnu.org/archive/html/bug-bash/2024-01/msg00137.html; this bug allows _some jobs_ that finished prior to the wait -n call to be returned).  I don't mean to disparage bash by pointing out a bug -- it is the only shell I'm aware of that has the feature I want and that community patiently engaged with me.  Thank you Chet Ramey and the rest of the bash community.

## Can We Do Better?
I'd like to improve the situation by building `wait -n` as a separate command.  I see a few benefits:
- this fits with unix's "do one thing and do it well" philosophy.  We already see the addition of options and features to bash's `wait` causing some confusion (internally managed queue/state; notion of the user being "notified" of subprocess completion) and bugs.
- separation of shell and commands speeds development by decoupling tools and constraints.  I can build such a utility in any language I want instead of C; I don't need to convince the Bash maintainers to accept any change.  I can experiment and get it wrong and not risk adding features and flags to a cornerstone project such as bash that later need to be adjusted.
- a separate utility would be useful for other shells that don't support `wait -n` such as posix sh (ash, ksh), zsh, and fish (whose documentation says it similarly waits for the _next_ job to finish; I want it to return the next job _to have finished_).

## What's the Problem?
This gets into Operation System process management.  I won't go too deep.

Operating System processes form a hierarchy with child-parent relationships.  A new process is created with the `fork` syscall, at which point the original parent is returned the process id (pid) of the new child, and the child is returned 0 to let it know it's the child (it can get its parent pid from a syscall if needed).  The parent process has historically been the only process allowed to wait for a its children to finish (the `wait` syscall; other processes may test for the existence of a process, e.g., on Linux by searching the /proc filesystem) and generally is still the only process that may retreive its children's exit codes.  The parent _must_ await its children's terminations as their exit codes are stored in the OS's process table -- if they weren't then the parent might miss a child process finishing and never learn its exit code.  Failure to await a child results in that child being done and having its resources freed (memory, file descriptors), but continuing to use a pid and keeping its place in the process table -- it becomes a "zombie."

`waitn` requires that a process wait until any of a number of _non-child_ processes terminates, hopefully without repeatedly polling.

And so we must look into non-portable OS system calls.  Thankfully, most popular OSes have recently introduced features to do just this:
- BSD and Darwin (MacOS) long ago gave us [kqueue](https://man.freebsd.org/cgi/man.cgi?kqueue)
- Windows offers [Process.WaitForExit()](https://learn.microsoft.com/en-us/dotnet/api/system.diagnostics.process.waitforexit?view=net-8.0#system-diagnostics-process-waitforexit) as part of its .NET API
- Linux recently introduced "pidfds," file descriptors referring to processes.  See [pidfd_open]("https://man7.org/linux/man-pages/man2/pidfd_open.2.html").  This syscall, needed to poll/epoll on pidfds, was introduced in Linux Kernel 5.3 in September, 2019.

## Let's get prototyping
I chose Go for my prototype.  My reasons:
- This work is an opportunity for me to learn.  It's one of _those_ projects.
- Go offers relatively direct access to system calls, and there are no general libraries I'm aware of in any language that expose pidfds.
- Go makes it easy to build (static linking, packages) and work with (defer statements, garbage collection)

Go may not be the right choice if I want my tool to be accepted by a large ecosystem of multiple OSes, but we're not there yet.

`waitn` uses syscalls from golang.org/x/sys/unix (see /internal/syscalls/pidfd.go):
- PidfdOpen -- create a pidfd or return an error that none could be found for a pid.
- Close -- close a file descriptor when done.
- Poll -- wait until at least one file descriptor from a slice/array is ready for reading and set/return some information about it.  Or, wait until a timeout.

That's it!  This was my first time calling syscalls directly instead of relying on language standard libraries.  It was quite easy for something I felt was arcane and intimidating.  The rest is "business logic"

## How Does It Work?
Let's revisit the earlier example about a Docker container.  We're going to start some processes, monitor for their terminations, and also listen for SIGTERM to forward it to the subprocesses.  This example can be found in /examples/forward_sigterm.sh.  All my scripts are bash and may not work with POSIX shell or zsh.

First, we need some logic to bridge the gap between our `waitn` and the shell's `wait`/`wait -n`.  Remember, `waitn` can tell you when processes finish, but it can't tell you their exit codes.  You still need to call `wait` from the process parent (shell).

see /examples/common.sh
```
# you can skip if waitn is in PATH
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
_waitn_path=$(realpath "$SCRIPT_DIR/../waitn")

# set up a temp file for waitn output, as well as an EXIT trap to rm it
# make sure to save and run any existing EXIT trap
_cur="$(trap -p EXIT | sed -nr "s/trap -- '(.*)' EXIT\$/\1/p")"
on_exit() {
    [ -n "$_waitn_temp_file" ] && rm -f "$_waitn_temp_file"
    [ -n "$_cur" ] && $_cur
}
trap on_exit EXIT
_waitn_temp_file=$(mktemp)

# set _waitn_path or if waitn is in PATH just call it
waitn_cmd() {
    "$_waitn_path" $@
}

# intended to match bash's "wait -n -p VARNAME pids..."
# do not pass "-n" flag
#
# note that we must take care with namerefs and locals.  If the "pid_var_name"
# passed to us aliases with any local var here then we end up setting the local
# instead of a global.
#
# all local variables will be prefixed with _waitn_.  The caller must not pass
# such a variable with -p and doing so will result in an error.
#
# rename this if the waitn command is in PATH
waitn() {
    # parse out any -p option
    local _waitn_pid_var_name
    if [ "$1" = "-p" ]; then
        _waitn_pid_var_name="$2"
        if [[ $myvar =~ ^_waitn_ ]]; then
            echo "-p varname begins with _waitn_.  Such names are restricted to prevent nameref collisions.  Use a different name"
            exit 1
        fi
        unset -n _waitn_pid_var_name
        shift 2
    fi

    # run the waitn command
    waitn_cmd $@ > $_waitn_temp_file &
    local _waitn_waitn_pid=$!
    local _waitn_finished_pid
    # wait for the waitn command
    wait -p _waitn_finished_pid $_waitn_waitn_pid
    local _waitn_wait_ret=$?

    if [ -n "$_waitn_finished_pid" ]; then
        # waitn completed

        # get the exit code of the returned pid
        local _waitn_pid=$(cat "$_waitn_temp_file")
        local _waitn_double_check_pid
        wait -p _waitn_double_check_pid $_waitn_pid
        local _waitn_ret=$?
        # assert that we found the same pid
        if [ -z "$_waitn_double_check_pid" ] || [ "$_waitn_double_check_pid" != "$_waitn_pid" ]; then
            echo "waiting to get exit code failed. pid: $_waitn_pid. found: $_waitn_double_check_pid"
            exit 1;
        fi

        # assign a variable for -p if provided
        if [ -n $_waitn_pid_var_name ]; then
            local -g -n _waitn_pid_var_ref="$_waitn_pid_var_name"
            _waitn_pid_var_ref="$_waitn_pid"
        fi

        # wait returns the exit code of the awaited process, which is the exit
        # code of the wait builtin
        return "$_waitn_ret"
    else
        # woke up due to a signal
        kill "$_waitn_waitn_pid"
        # wait until waitn definitely returns.  I want to reuse the temp file
        # without having to recreate it.  We can't risk the old waitn being
        # around and overwriting the file.
        while true; do
            wait -p _waitn_finished_pid $_waitn_waitn_pid
            [ -n "$_waitn_finished_pid" ] && break
        done

        # return the original wait_ret, which indicates which signal (128 + signal num)
        return "$_waitn_wait_ret"
    fi
}
```

OK, that's longer than expected.  There are a few things going on:

`waitn` must be called as a background job so that it can be awaited (shell `wait`), which lets us handle signals immediately.  If shell is running any other command while a signal arrives it will wait for the command to finish before running the handler.  `wait` is _the_ tool for concurrency.

Because `waitn` is running as a background job, but we need to collect its stdout to get the pid of the terminated process, we're going to redirect it to a temporary file.  We need to clean up the temporary file when we're done.  This is accomplished in an EXIT handler (trap).  Since the caller may have also defined an EXIT trap we don't want to clobber it, so we read the current trap handler and run it as well (I'm starting to think maybe shell _isn't_ the simplest tool for this job, but I'm invested).

The waitn function will accept a variable name into which it assigns the pid of the terminated process (`-p VARNAME`).  The shell feature which allows this a _nameref_ (`declare -n`).  This can also be done with `eval` expansion which is discouraged due to security risks.  Even with a nameref the assignment still occurs with simple variable name expansion.  As a result, if the caller passes a variable name that collides with a variable within the function then it assigns the function variable instead of the caller's global variable.  I name _all_ function variables `_waitn_...` and check that the provided variable name doesn't match this pattern.  Shell is looking less and less simple, but at least this is the code that you'll _call_ and you don't have to _write_ again.

The `waitn` function calls the `waitn` executable as a background function, `wait`s for it, determines whether it returned due to a signal or because a process terminated, gets the pid of the terminated process, and then `wait`s for the _that_ pid to get an exit code.  It then assigns, via nameref, any `-p VARNAME` provided.  It's convoluted but it provides the same semantics as bash's `wait -n`.

Now let's use it.  See examples/forward_sigterm.sh
```
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source $(realpath "$SCRIPT_DIR/common.sh")

# start some mock jobs
#
# we'll sleep 2 and then kill
declare -A pids
# finishes prior to kill
{ sleep 1; exit 1; } &
pids[$!]=1
# still running when killed
{ sleep 3; exit 2; } &
pids[$!]=2

# waits until a job finishes or a signal arrives
# we kill from the SIGTERM handler, so here we just skip wait waking up due to
# signal.  We're return 1 to indicate this, but we don't actually use it.
wait_for_job() {
    local finished_pid
    waitn -p finished_pid "${!pids[@]}"
    wait_ret=$?
    # if finished_pid isn't assigned then we woke up due to signal.  Return
    [ -z "$finished_pid" ] && return 1
    echo "FINISHED ${pids[$finished_pid]} exit code $wait_ret @${SECONDS}"
    unset pids[$finished_pid]
}

handled_term=false
term_handler() {
    handled_term=true
    echo "killing jobs from handler @${SECONDS}"
    kill -TERM "${!pids[@]}"
}
trap term_handler TERM

sleep 2 && echo "killing bash! @${SECONDS}" && kill -TERM $$ &

while [ ${#pids[@]} -gt 0 ]; do
    wait_for_job
done

# epilogue: if we handled SIGTERM then term self
if $handled_term; then
    # reset
    trap - TERM
    kill -TERM $$
fi

# bash should print "Terminated" and end.
# you can also query $? for 143 (128 + 15 where SIGTERM is 15)
```

This is a bit more straightforward:
- start some mock jobs that simply sleep and exit
- store job pids as the keys of an associate array.  This is essentially a set, and makes it easier to later remove pids of terminated jobs.
- define a function to wait for a job or a signal by calling the `waitn` function.  _Now_ the code looks simple because we did all the legwork in the command and function.  The function simply prints the pid and exit code.
- define a SIGTERM handler to kill the remaining jobs.
- start a subprocess to kill _this shell_ at some point in the future.
- wait for all jobs to finish in a loop.  `wait_for_job` will remove pids from `pids` as processes terminate.  It also wakes up immediately on SIGTERM, allowing `term_handler` to run before looping again to monitor the subprocesses.
- finally, if this shell was sent SIGTERM then reset the SIGTERM handler and send the signal again, to itself.  This should stop the process immediately with the exit code and signal state that the original signaller might be expecting.

## Conclusion
By using recent OS syscalls I replicated, and hopefully improved upon, bash's `wait -n` command to monitor and manage suprocesses.  This decouples it from the shell and makes it available to other shells and programming languages.  Shell `wait` and `trap` offer surprisingly simple primitives for IPC/process interaction.  For simple cases shell is easier to understand (assuming you're familiar with these mechanisms) than some high level programming languages.

As an aside, I like Go for its concurrency tools, subprocess management, and signal handling (especially signal handling).  But I find Go very verbose, particularly error handling.  This can be a strength for long-maintained projects -- invest in _thinking about_ and _writing_ the complex parts (i.e., error handling) and you'll spend less time down the road _investigating_ and _fixing_ those parts.  If I were deploying this example into a critical production service I might spend the time and use Go.  For simple tasks I want simple tools, and shell still fits the bill.