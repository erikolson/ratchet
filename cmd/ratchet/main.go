// Command ratchet makes "verified" a fact rather than a claim: declare what
// verification means in a manifest, and the enforcement is derived from it.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/erikolson/ratchet/internal/check"
	"github.com/erikolson/ratchet/internal/doctor"
	"github.com/erikolson/ratchet/internal/gate"
	"github.com/erikolson/ratchet/internal/gitx"
	"github.com/erikolson/ratchet/internal/hooks"
	"github.com/erikolson/ratchet/internal/oracles"
	"github.com/spf13/cobra"
)

// exitError carries an explicit process exit code up to main. check/gate produce
// 0 (pass/allow) / 1 (fail) / 2 (error); anything else that reaches main is a
// couldn't-run condition and exits 3 (never a verdict about the code).
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func main() {
	err := newRootCmd().Execute()

	var ec *exitError
	if errors.As(err, &ec) {
		os.Exit(ec.code)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ratchet:", err)
		os.Exit(3)
	}
	os.Exit(0)
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "ratchet",
		Short:         "certainty in change — make \"verified\" a fact, not a claim",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newCheckCmd(),
		newDoctorCmd(),
		newGateCmd(),
		newDiffOraclesCmd(),
		newInstallHooksCmd(),
		newGateHookCmd(),
	)
	return root
}

// repoRoot resolves the git worktree root of the current directory.
func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := gitx.RepoRoot(cwd)
	if err != nil {
		return "", fmt.Errorf("not a git repository (ratchet requires git): %w", err)
	}
	return root, nil
}

func newCheckCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "check [capability...]",
		Short: "run declared capabilities, emit a verdict per capability, write receipts",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			res, err := check.Run(check.Options{
				RepoRoot: root, Only: args, JSON: jsonOut,
				Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			return &exitError{code: res.ExitCode}
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit verdicts as JSON to stdout, tool output to stderr")
	return cmd
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "verify the verifier — calibrate each oracle against ratified mutation probes",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			rep, err := doctor.Run(doctor.Options{
				RepoRoot: root, Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			return &exitError{code: rep.ExitCode}
		},
	}
}

func newGateCmd() *cobra.Command {
	var action string
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "run the full check and deny (nonzero) if anything is red — the enforcement primitive",
		Long: "The single primitive both hook surfaces invoke. Runs every declared " +
			"capability (never a subset), records a block/allow decision referencing the " +
			"check verdicts by identity, and fails closed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			res, err := gate.Run(gate.Options{
				RepoRoot: root, Action: action,
				Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			return &exitError{code: res.ExitCode}
		},
	}
	cmd.Flags().StringVar(&action, "action", "commit", "label for what is being gated, recorded in the receipt")
	return cmd
}

func newDiffOraclesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff-oracles <base-ref>",
		Short: "report oracle-hash changes vs a base ref (added=tightening silent; changed/removed=review)",
		Long: "Compare the working-tree manifest's oracle hashes against the manifest at " +
			"<base-ref> (the protected branch). Tightening is silent; weakening and removal " +
			"alarm. Informational with a signal exit code — wire it as a required CI check or not.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			rep, err := oracles.Diff(root, args[0], cmd.OutOrStdout())
			if err != nil {
				return err
			}
			return &exitError{code: rep.ExitCode}
		},
	}
}

func newInstallHooksCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install-hooks",
		Short: "install the local gate: Claude PreToolUse hook + git pre-commit hook",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			bin, err := os.Executable()
			if err != nil {
				return err
			}
			res, err := hooks.Install(hooks.Options{RepoRoot: root, Binary: bin})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, p := range res.Wrote {
				if rel, e := filepath.Rel(root, p); e == nil {
					fmt.Fprintf(out, "✓ wrote %s\n", rel)
				} else {
					fmt.Fprintf(out, "✓ wrote %s\n", p)
				}
			}
			fmt.Fprintln(out, "\nLocal gate installed — fast feedback and a receipt, not a guarantee against a")
			fmt.Fprintln(out, "determined agent. Prevention lives off-machine: run the same check in CI, and")
			fmt.Fprintln(out, "use `ratchet diff-oracles <base>` on the protected branch to catch weakenings.")
			return &exitError{code: 0}
		},
	}
}

var gitCommitRe = regexp.MustCompile(`\bgit\b[\s\S]*\bcommit\b`)

// newGateHookCmd bridges the Claude Code PreToolUse protocol: read the tool call
// from stdin, and if it is a git commit, run the gate. Exit 2 blocks the tool call
// (Claude Code convention); exit 0 allows. Best-effort git-commit detection — a
// determined agent can route around it, which is exactly why prevention lives
// off-machine (ADR-0008).
func newGateHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "__gate-hook",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, _ := io.ReadAll(os.Stdin)
			var input struct {
				ToolName  string `json:"tool_name"`
				ToolInput struct {
					Command string `json:"command"`
				} `json:"tool_input"`
			}
			_ = json.Unmarshal(data, &input)

			if !gitCommitRe.MatchString(input.ToolInput.Command) {
				return &exitError{code: 0} // not a commit — allow
			}
			root, err := repoRoot()
			if err != nil {
				return &exitError{code: 0} // not a git repo — not ours to gate
			}
			if _, err := os.Stat(filepath.Join(root, "ratchet.yaml")); err != nil {
				return &exitError{code: 0} // repo is not ratchet-governed — allow
			}
			res, err := gate.Run(gate.Options{
				RepoRoot: root, Action: "git commit",
				Stdout: os.Stderr, Stderr: os.Stderr, // shown to the model on block
			})
			if err != nil {
				fmt.Fprintln(os.Stderr, "ratchet:", err)
				return &exitError{code: 2}
			}
			if res.Blocked {
				return &exitError{code: 2}
			}
			return &exitError{code: 0}
		},
	}
}
