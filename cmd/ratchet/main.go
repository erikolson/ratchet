// Command ratchet makes "verified" a fact rather than a claim: declare what
// verification means in a manifest, and the enforcement is derived from it.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/erikolson/ratchet/internal/check"
	"github.com/erikolson/ratchet/internal/gitx"
	"github.com/spf13/cobra"
)

// exitError carries an explicit process exit code up to main. check produces
// 0 (pass) / 1 (fail) / 2 (error); anything else that reaches main is a
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
	root.AddCommand(newCheckCmd())
	return root
}

func newCheckCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "check [capability...]",
		Short: "run declared capabilities, emit a verdict per capability, write receipts",
		Long: "Run the capabilities declared in ratchet.yaml (all of them, or the named " +
			"subset), emit a normalized verdict per capability to .ratchet/verdicts.jsonl, " +
			"and exit 0 if all pass, 1 if any fail, 2 if any error.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			root, err := gitx.RepoRoot(cwd)
			if err != nil {
				return fmt.Errorf("not a git repository (ratchet requires git): %w", err)
			}
			res, err := check.Run(check.Options{
				RepoRoot: root,
				Only:     args,
				JSON:     jsonOut,
				Stdout:   cmd.OutOrStdout(),
				Stderr:   cmd.ErrOrStderr(),
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
