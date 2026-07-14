package verdict

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

// OracleSpec is the fully-resolved definition of a capability's oracle. Its Hash
// is one of the four identity components (ADR-0001): weaken the command, the exit
// code sets, the timeout, or the schema version, and the identity changes.
//
// pass/fail are code SETS (order-insensitive); argv defines the process
// (order-significant). prepare participates only for calibration verdicts
// (IncludePrepare true), because prepare never runs for check.
type OracleSpec struct {
	Adapter        string
	Version        int
	Argv           []string
	Pass           []int
	Fail           []int
	Timeout        time.Duration
	IncludePrepare bool
	Prepare        []string
}

// Hash returns the hex sha256 of a canonical, length-delimited encoding.
func (s OracleSpec) Hash() string {
	sum := sha256.Sum256(s.canonical())
	return hex.EncodeToString(sum[:])
}

func (s OracleSpec) canonical() []byte {
	var b bytes.Buffer
	b.WriteString("ratchet-oracle-v1\n")
	fieldStr(&b, "adapter", s.Adapter)
	fieldInt(&b, "version", int64(s.Version))
	fieldStrs(&b, "argv", s.Argv)         // order significant
	fieldInts(&b, "pass", sortedUniq(s.Pass)) // set
	fieldInts(&b, "fail", sortedUniq(s.Fail)) // set
	fieldInt(&b, "timeout_ns", int64(s.Timeout))
	// A present marker distinguishes "prepare absent" (check) from
	// "prepare present but empty" (calibration with no prepare step).
	fieldInt(&b, "prepare_present", boolToInt(s.IncludePrepare))
	if s.IncludePrepare {
		fieldStrs(&b, "prepare", s.Prepare) // order significant
	}
	return b.Bytes()
}

// Length-delimited field writers: every string carries its byte length so that
// element boundaries are unambiguous (["a","b"] cannot collide with ["ab"]).

func fieldStr(b *bytes.Buffer, key, val string) {
	fmt.Fprintf(b, "%s=%d:", key, len(val))
	b.WriteString(val)
	b.WriteByte('\n')
}

func fieldInt(b *bytes.Buffer, key string, n int64) {
	fmt.Fprintf(b, "%s=%d\n", key, n)
}

func fieldStrs(b *bytes.Buffer, key string, xs []string) {
	fmt.Fprintf(b, "%s=%d[\n", key, len(xs))
	for _, x := range xs {
		fmt.Fprintf(b, "%d:", len(x))
		b.WriteString(x)
		b.WriteByte('\n')
	}
	b.WriteString("]\n")
}

func fieldInts(b *bytes.Buffer, key string, xs []int) {
	fmt.Fprintf(b, "%s=%d[\n", key, len(xs))
	for _, x := range xs {
		fmt.Fprintf(b, "%d\n", x)
	}
	b.WriteString("]\n")
}

func sortedUniq(xs []int) []int {
	if len(xs) == 0 {
		return nil
	}
	cp := append([]int(nil), xs...)
	sort.Ints(cp)
	out := cp[:1]
	for _, x := range cp[1:] {
		if x != out[len(out)-1] {
			out = append(out, x)
		}
	}
	return out
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
