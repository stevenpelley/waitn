### Example usage from Bash (should be similar for other shells)
Note that most of these would work identically with bash's `wait -n` today.  With waitn you can do the same in zsh/sh!  All of these examples are copied from the shell scripts in these directories.  I find them easier to browse in a single document.

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