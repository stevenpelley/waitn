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