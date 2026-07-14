// Package shlex tokenizes a capability's `run` string into an argv vector using
// POSIX shell-word rules — quoting and backslash escaping, but NO expansion and
// NO execution.
//
// It deliberately rejects shell metacharacters (ADR-0003): a capability is
// exactly one process, so `&&`, `||`, pipes, redirects, subshells, and
// expansion cannot be written. Composition belongs in the manifest (declare
// another capability) or a version-controlled script, never in a run string.
//
// The tokenizer targets the common POSIX subset that appears in real verify
// commands; it is not a full shell parser.
package shlex

import "fmt"

// MetacharError reports a shell metacharacter that ratchet refuses to interpret.
type MetacharError struct {
	Char string // the offending character, e.g. "&", "|", "$"
}

func (e *MetacharError) Error() string {
	return fmt.Sprintf("shell metacharacter %q not allowed: a capability is one process — "+
		"declare another capability or move the pipeline into a script", e.Char)
}

// SyntaxError reports malformed quoting or a dangling escape.
type SyntaxError struct{ msg string }

func (e *SyntaxError) Error() string { return e.msg }

type state int

const (
	stateNormal state = iota
	stateSingle
	stateDouble
)

// activeUnquoted are metacharacters that carry shell meaning outside quotes.
func activeUnquoted(r rune) bool {
	switch r {
	case '&', '|', ';', '<', '>', '(', ')', '$', '`', '\n':
		return true
	}
	return false
}

// Split tokenizes s. It returns the argv, or a *MetacharError / *SyntaxError.
// An empty or whitespace-only string yields a nil slice with no error;
// callers that require a non-empty command (the manifest validator) enforce
// that separately.
func Split(s string) ([]string, error) {
	var tokens []string
	var buf []rune
	started := false // current token has begun (may be empty via "" or '')
	escaped := false
	st := stateNormal

	flush := func() {
		if started {
			tokens = append(tokens, string(buf))
			buf = buf[:0]
			started = false
		}
	}

	for _, r := range s {
		if escaped {
			buf = append(buf, r)
			escaped = false
			started = true
			continue
		}
		switch st {
		case stateNormal:
			switch {
			case r == '\\':
				escaped = true
				started = true
			case r == '\'':
				st = stateSingle
				started = true
			case r == '"':
				st = stateDouble
				started = true
			case r == ' ' || r == '\t' || r == '\r':
				flush()
			case activeUnquoted(r):
				return nil, &MetacharError{Char: string(r)}
			default:
				buf = append(buf, r)
				started = true
			}
		case stateSingle:
			// Everything is literal inside single quotes, including
			// metacharacters and backslashes.
			if r == '\'' {
				st = stateNormal
			} else {
				buf = append(buf, r)
			}
		case stateDouble:
			switch r {
			case '"':
				st = stateNormal
			case '\\':
				escaped = true
			case '$', '`':
				// Expansion is active inside double quotes in a real shell;
				// ratchet never expands, so reject rather than silently
				// diverge from shell semantics.
				return nil, &MetacharError{Char: string(r)}
			default:
				buf = append(buf, r)
			}
		}
	}

	if escaped {
		return nil, &SyntaxError{msg: "dangling backslash at end of run string"}
	}
	if st != stateNormal {
		return nil, &SyntaxError{msg: "unterminated quote in run string"}
	}
	flush()
	return tokens, nil
}
