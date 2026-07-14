package shlex

import (
	"errors"
	"reflect"
	"testing"
)

func TestSplit_Valid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "pytest -q", []string{"pytest", "-q"}},
		{"dot arg", "mypy .", []string{"mypy", "."}},
		{"empty", "", nil},
		{"only spaces", "   ", nil},
		{"collapses spaces", "  a   b  ", []string{"a", "b"}},
		{"single quoted space", "ruff check 'src/my dir'", []string{"ruff", "check", "src/my dir"}},
		{"double quoted space", `echo "hello world"`, []string{"echo", "hello world"}},
		{"backslash escaped space", `a b\ c`, []string{"a", "b c"}},
		{"escaped metachar is literal", `grep \& file`, []string{"grep", "&", "file"}},
		{"metachar literal in single quotes", "grep '||' file", []string{"grep", "||", "file"}},
		{"dollar literal in single quotes", `grep '$foo' file`, []string{"grep", "$foo", "file"}},
		{"escaped quote in double quotes", `echo "a\"b"`, []string{"echo", `a"b`}},
		{"adjacent quoted and bare concatenate", `foo'bar'baz`, []string{"foobarbaz"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Split(tc.in)
			if err != nil {
				t.Fatalf("Split(%q) unexpected error: %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Split(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSplit_RejectsMetacharacters(t *testing.T) {
	// Each of these must be rejected at tokenization so a capability stays
	// exactly one process (ADR-0003). The offending character is reported.
	cases := []struct {
		name string
		in   string
		char string
	}{
		{"and-and", "mypy . && pytest", "&"},
		{"single-amp", "pytest & echo", "&"},
		{"or-or", "pytest || true", "|"},
		{"pipe", "pytest | tee log", "|"},
		{"semicolon", "a ; b", ";"},
		{"redirect out", "pytest > out", ">"},
		{"redirect in", "cat < f", "<"},
		{"dollar unquoted", "echo $HOME", "$"},
		{"dollar in double quotes", `echo "$HOME"`, "$"},
		{"backtick", "echo `date`", "`"},
		{"subshell open", "echo (x)", "("},
		{"subshell close", "echo x)", ")"},
		{"newline", "a\nb", "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Split(tc.in)
			if err == nil {
				t.Fatalf("Split(%q) = nil error, want metacharacter rejection", tc.in)
			}
			var me *MetacharError
			if !errors.As(err, &me) {
				t.Fatalf("Split(%q) error = %v, want *MetacharError", tc.in, err)
			}
			if me.Char != tc.char {
				t.Fatalf("Split(%q) rejected %q, want %q", tc.in, me.Char, tc.char)
			}
		})
	}
}

func TestSplit_RejectsUnterminatedQuote(t *testing.T) {
	for _, in := range []string{`echo 'unterminated`, `echo "unterminated`, `trailing\`} {
		if _, err := Split(in); err == nil {
			t.Fatalf("Split(%q) = nil error, want unterminated/dangling error", in)
		}
	}
}
