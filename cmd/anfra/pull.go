package main

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/cliauth"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/publish"
	"github.com/holistics/anfra/shared/apperr"
)

func newPullCmd() *cobra.Command {
	var o publish.PullOptions
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Bring the changes others published to this repo's org into this folder",
		Example: "  anfra pull\n" +
			"  anfra pull --take org|local <path>...\n" +
			"  anfra pull --abort",
		Long: "Bring the changes others published to this repo's org into this folder.\n\n" +
			"What pull does:\n" +
			"  - Changes that don't overlap with yours are merged into your files for you.\n" +
			"  - Where you and the org changed the same lines, the file gets conflict markers, with both\n" +
			"    versions, for you to resolve.\n" +
			"  - It only writes the files a publish sends (.aml files, and everything under apps/). It keeps\n" +
			"    a copy of every file it changes, in .anfra/publish/, so you can undo it.\n\n" +
			"After a pull with conflicts:\n" +
			"  - Edit each conflicted file to keep the right changes, and remove the markers. Or delete the\n" +
			"    file, if deleting it is right.\n" +
			"  - To keep one version of a whole file, run `anfra pull --take org <path>` for the org's, or\n" +
			"    `--take local <path>` for this folder's.\n" +
			"  - Then run `anfra validate`, and `anfra publish`.\n" +
			"  - To undo the pull instead, run `anfra pull --abort`. Edits made after the pull to the files it\n" +
			"    changed are lost.\n\n" +
			"Run pull in a folder that `anfra init` set up.",
		Args: func(cmd *cobra.Command, args []string) error {
			if o.Take == "" && len(args) > 0 {
				return clierr.Precondition("anfra pull takes paths only with --take: `anfra pull --take org|local <path>...`")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Paths = args
			if o.Take != "" && o.Abort {
				return clierr.Precondition("--take and --abort can't be used together: --take resolves conflicts, --abort undoes the pull.")
			}
			terminal := newAuthTerminal(cmd.InOrStdin(), cmd.ErrOrStderr())
			// Progress is for a person at a terminal; warnings and errors go to
			// stderr whoever reads them.
			progress := io.Discard
			var selectOrg func(context.Context) (cloudapi.Org, error)
			var recoverOrg func(context.Context, cloudapi.Org, error) (cloudapi.Org, error)
			if terminal.interactive {
				progress = cmd.ErrOrStderr()
				selectOrg = terminal.selectPublishOrg
				recoverOrg = terminal.recoverPublishOrg
			}
			logger, closeLog := repoLog()
			defer closeLog()
			res, err := publish.Pull(cmd.Context(), ".", o, publish.Env{
				ErrOut:     progress,
				Logger:     logger,
				Warn:       func(code, message string) { printWarning(cmd.ErrOrStderr(), code, message) },
				OrgName:    storedOrgName(cmd.Context()),
				Credential: (cliauth.Manager{}).Authorize,
				SelectOrg:  selectOrg,
				RecoverOrg: recoverOrg,
				UserAgent:  "anfra/" + meta.Version,
			})
			if err != nil && cmd.Context().Err() != nil {
				return apperr.New(clierr.Interrupted, "Interrupted: not pulled.")
			}
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	markPrettyOutput(cmd)
	resultIs[publish.PullResult](cmd)
	markInteractable(cmd)
	f := cmd.Flags()
	f.StringVar(&o.SetOrg, "set-org", "", "the org to pull from, by its address, https://<server>/o/<org>. It's saved as the repo's org (.anfra/config.yml org.url)")
	f.StringVar(&o.Take, "take", "", `keep one version of the whole file at each path given, for files with a conflict: "org" or "local"`)
	f.BoolVar(&o.Abort, "abort", false, "undo the last pull, putting back every file it changed. Edits made after the pull to those files are lost")
	return cmd
}
