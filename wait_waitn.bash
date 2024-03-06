#!/usr/bin/env bash
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

wait_cmd() {
    local _waitn_path=$(realpath "$SCRIPT_DIR/waitn")
    "$_waitn_path" $@
}

wait_cmd_get_pid() {
    cat
}