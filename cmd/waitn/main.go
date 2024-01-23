package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/stevenpelley/waitn/internal/sys"
)

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		fmt.Fprintln(os.Stderr, "no pids provided")
		os.Exit(127)
	}
	pids := make([]int, 0)
	for _, arg := range flag.Args() {
		pid, err := strconv.Atoi(arg)
		if err != nil {
			panic(err)
		}
		pids = append(pids, pid)
	}

	pidfds := make([]*sys.Pidfd, len(pids))
	for i, pid := range pids {
		pidfd, err := sys.Open(pid)
		if err != nil {
			panic(err)
		}
		defer pidfd.Close()
		pidfds[i] = pidfd
	}

	readyFds, err := sys.Poll(pidfds, -1)
	if err != nil {
		panic(err)
	}

	readyPids := make([]int, len(readyFds))
	for i := range readyFds {
		readyPids[i] = readyFds[i].Pid
	}

	fmt.Printf("ready pids: %v\n", readyPids)
}
