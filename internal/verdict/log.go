package verdict

import (
	"fmt"
	"os"
)

// maxLine bounds a serialized verdict so a single O_APPEND write is atomic
// (ADR-0005). Verdicts are small and fixed-shape under the exit adapter; this is
// a guard, not a budget to spend.
const maxLine = 4096

// Append writes v as one newline-terminated JSON line to the verdict log at path,
// creating it if absent. The log is append-only; callers never rewrite it.
func Append(path string, v Verdict) error {
	line, err := Marshal(v)
	if err != nil {
		return err
	}
	if len(line)+1 > maxLine {
		return fmt.Errorf("verdict line is %d bytes, exceeds %d (atomic-append guard)", len(line)+1, maxLine)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}
