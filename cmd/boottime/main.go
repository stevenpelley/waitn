package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// command to retrieve time using linux syscall.
// this is necessary to use the same clock that reading the start time of
// processes uses.

func main() {
	var clockId int32
	if len(os.Args) < 2 {
		clockId = unix.CLOCK_BOOTTIME
	} else if len(os.Args) == 2 {
		clockName := os.Args[1]
		clockId = getClockId(clockName)
	} else if len(os.Args) > 2 {
		fmt.Fprintf(os.Stderr, "unexpected arguments\n")
		os.Exit(1)
	}

	var ts unix.Timespec
	unix.ClockGettime(clockId, &ts)
	// a signed int64 can encode 292 years of ns.  Let's just return total ns
	ns := uint64(ts.Nsec) + (1_000_000_000 * uint64(ts.Sec))
	fmt.Println(ns)
	//fmt.Printf("%v.%v\n", ts.Sec, ts.Nsec)
}

func getClockId(clockName string) int32 {
	var clockId int32
	switch clockName {
	case "CLOCK_BOOTTIME":
		clockId = unix.CLOCK_BOOTTIME
	case "CLOCK_MONOTONIC":
		clockId = unix.CLOCK_MONOTONIC
	case "CLOCK_MONOTONIC_COARSE":
		clockId = unix.CLOCK_MONOTONIC_COARSE
	case "CLOCK_MONOTONIC_RAW":
		clockId = unix.CLOCK_MONOTONIC_RAW
	default:
		fmt.Fprintf(os.Stderr, "unexpected clock: %v\n", clockName)
		os.Exit(1)
	}
	return clockId
}
