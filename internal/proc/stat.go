package proc

// This is currently unreferenced code.
// It was intended to provide a safe means of determining if a pid refers to the
// correct process (as pids may be reused) by using the pids starttime read from
// /proc/pid/stat.  It got complicated.  There are enough other cases in linux
// that need to address the reuse of pids, I'm not going to try to solve it
// here.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func IsCorrectProcess(pid int, processStartTime uint64, globalStartTime uint64, clkTckHz uint64) (bool, error) {
	if processStartTime == 0 && globalStartTime == 0 {
		return true, nil
	}

	s, err := os.ReadFile(fmt.Sprintf("/proc/%v/stat", pid))
	if err != nil {
		// assume the process is gone
		// TODO: examine the errors more rigorously.  Examine if the
		// /proc/pid directory itself exists, implying that the process
		// is still running.
		return false, nil
	}

	statStartTime, err := readStatStarttime(string(s))
	if err != nil {
		return false, err
	}

	if processStartTime != 0 {
		return processStartTime == statStartTime, nil
	}

	// use globalStartTime
	// convert stat starttime to ns based on clkTckHz
	factor := 1_000_000_000 / clkTckHz
	startTime := statStartTime * factor
	return startTime < globalStartTime, nil
}

func readStatStarttime(contents string) (uint64, error) {
	// get the starttime from the stat file
	// it is the 22nd field.  The 2nd field is the filename of the executable in
	// parenthesis.  It might have spaces and nested parenthesis and is the
	// only field that may be non-alphanumeric.  We will search for the
	// right-most ") " and assume the 3rd field starts immediately after.  Then
	// we find the 22nd overall field.

	lastIdx := strings.LastIndex(contents, ") ")
	if lastIdx == -1 {
		return 0, fmt.Errorf(
			"read proc stat starttime: \") \" not found (expected in field 2 of file).  Contents: %v",
			contents)
	}
	fieldsThreeAndUp := contents[lastIdx+2:]
	fields := strings.Split(fieldsThreeAndUp, " ")
	if len(fields) < 20 {
		return 0, fmt.Errorf(
			"read proc stat starttime: fewer fields than expected found after close parenthesis (assumed to be field 2).  Contents: %v",
			contents)
	}

	s := fields[19]
	statStartTime, err := strconv.ParseUint(s, 10, 0)
	if err != nil {
		return 0, fmt.Errorf(
			"read proc stat starttime: parsing starttime string.  Contents: %v: %w",
			contents,
			err)
	}
	return statStartTime, nil
}
