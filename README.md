# waitn
Provides bash-like "wait -n" functionality as a separate command and with some semantic differences.

## Bash's wait

Bash provides `wait` options `-n` and `-p` to help with coordinating multiple subprocesses.
A brief primer on wait's behavior:
- `wait` with no options or arguments waits until all child processes terminate and returns 0.
- `wait <pid>...` waits until all listed pids terminate and returns the exit code of the last process listed.
- `wait -n` waits until the next child process terminates and returns its error code.
- `wait -n <pid>...` waits until the next listed child process terminates and returns its error code.
- `-p VARNAME` added to `wait` or `wait -n` assigns to variable `VARNAME` the pid of the completed process.  This is useful for `wait -n` to determine which process finished, but also for all wait variants to help distinguish between a process completing or wait returning due to a trapped signal.  When wait returns due to a trapped signal the indicated variable will be the empty string.
`wait` will also returned on any trapped/handled signal, returning with exit code 128+signal number (e.g., 120 for SIGINT, 143 for SIGTERM).  Note that child processes may themselves return the same exit code, and so `-p` distinguishes between these cases.

A complication of `wait -n` is that it only returns jobs that finish after it is called.  There may be races where jobs finish prior to the first call to `wait -n`, or finish between calls to `wait -n`.  Such jobs will not be returned.  (There is a wide misunderstanding and at least one bug, see https://lists.gnu.org/archive/html/bug-bash/2024-01/msg00137.html; this bug allows _some jobs_ that finished prior to the wait -n call to be returned).

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

### Implementation
Linux now provides https://man7.org/linux/man-pages/man2/pidfd_open.2.html
pidfds are file descriptors that are opened either with a call to `clone` or by pid with `pidfd_open`.  Their original purpose is to avoid unsafe of `signal` where a process terminates, its pid is reused, and the signal sent to the incorrect process of the same pid.  One can open a pidfd to a child process and if you haven't awaited that process you can guarantee that it refers to the correct process (even if it has terminated it is a zombie process since it hasn't been awaited).  From that point you may safely signal the process using the pidfd and it will never alias to another process.  Pidfds also allow polling/epolling the status of a process -- when the process terminates the fd is available for reading.

Note that we may still alias pids and accidentally wait on a process with a reused pid.  This would cause us to block longer than expected.  In the future this can be addressed by locating a start timestamp for each process and passing it to wait, but this increases complexity substantially and this is a Linux-wide problem; I'm not going to solve it here.

### Example usage from Bash (should be similar for other shells)

#### Drop in replacement for "wait -n"
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

#### Block and handle processes one at a time
see examples/simple.sh
```
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source $(realpath "$SCRIPT_DIR/common.sh")

declare -A pids
# start jobs, populate $pids
for task in {1..3}; do
    start_job $task
done

wait_for_job() {
    local finished_pid
    waitn -p finished_pid "${!pids[@]}"
    wait_ret=$?
    echo "FINISHED ${pids[$finished_pid]} exit code $wait_ret @${SECONDS}"
    unset pids[$finished_pid]
}

while [ ${#pids[@]} -gt 0 ]; do
    wait_for_job
done
```

#### Start jobs with concurrency limit
see examples/limit_concurrency.sh
```
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source $(realpath "$SCRIPT_DIR/common.sh")

declare -A pids
limit=3
remaining_tasks=($(seq 0 9))

# modifies remaining_tasks and pids
# returns 0 if a new task was started and 1 otherwise.
# this does not test the concurrency limit
start_task_if_remaining() {
    [ "${#remaining_tasks[@]}" -eq 0 ] && return 1
    task=${remaining_tasks[0]}
    remaining_tasks=("${remaining_tasks[@]:1}")
    # start job, populate $pids
    start_job $task
}

# identical to simple.sh
wait_for_job() {
    local finished_pid
    waitn -p finished_pid "${!pids[@]}"
    wait_ret=$?
    echo "FINISHED ${pids[$finished_pid]} exit code $wait_ret @${SECONDS}"
    unset pids[$finished_pid]
}

while [ ${#remaining_tasks[@]} -gt 0 ] || [ ${#pids[@]} -gt 0 ]; do
    # start processes until we get up to the limit
    while [ ${#pids[@]} -lt $limit ] && start_task_if_remaining ; do : ; done
    wait_for_job
done
```

#### Forward SIGTERM
see examples/forward_sigterm.sh
```
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source $(realpath "$SCRIPT_DIR/common.sh")

# we'll sleep 2 and then kill
declare -A pids
# finishes prior to kill
{ sleep 1; exit 1; } &
pids[$!]=1
# still running when killed
{ sleep 3; exit 2; } &
pids[$!]=2

# we kill from the SIGTERM handler, so here we just skip wait waking up due to
# signal.  We're return 1 to indicate this, but we don't actually use it.
wait_for_job() {
    local finished_pid
    waitn -p finished_pid "${!pids[@]}"
    wait_ret=$?
    # this line is new relative to simple.sh
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

if $handled_term; then
    # reset
    trap - TERM
    # term self after having forwarded and joining all children
    kill -TERM $$
fi

# bash should print "Terminated" and end.
# you can also query $? for 143 (128 + 15 where SIGTERM is 15)
```

#### Job Finish Order
see examples/finish_order.sh.
Note that the case where a job terminates due to SIGTERM prior to the call to
waitn doesn't work in bash as of the time of this writing.
```
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source $(realpath "$SCRIPT_DIR/common.sh")

# we want jobs that exit normally before and after we call waitn, and jobs that
# terminate due to SIGTERM before and after waitn
# we'll call waitn at time 3
declare -A pids

{ sleep 1; exit 1; } &
pids[$!]=1

# to be killed at time 2
{ sleep 10; exit 2; } &
kill_at_2=$!
pids[$kill_at_2]=2
{ sleep 2; kill $kill_at_2; } &

{ sleep 4; exit 3; } &
pids[$!]=3

# to be killed at time 5
{ sleep 10; exit 4; } &
kill_at_5=$!
pids[$kill_at_5]=4
{ sleep 5; kill $kill_at_5; } &

sleep 3

# same as simple.sh
wait_for_job() {
    local finished_pid
    waitn -p finished_pid "${!pids[@]}"
    wait_ret=$?
    echo "FINISHED ${pids[$finished_pid]} exit code $wait_ret @${SECONDS}"
    unset pids[$finished_pid]
}

while [ ${#pids[@]} -gt 0 ]; do
    wait_for_job
done
```

## Future
Take a look at libkqueue as a library to make this portable.  It doesn't look terribly well supported, but may simply be complete.
I could also call bsd's libc from cgo directly.  Windows appears to have the needed syscalls as well.
Consider whether Go is the right tool.  It's convenient and has remarkably easy access to syscalls for a high level language, but may produce large binaries, have quirks with system calls interacting with goroutines/threads, or otherwise not be accepted by the old school gnu/core utils community.  I suspect Rust, if anything, would be more appropriate, but this is typically the domain of C.