package cli

import (
	"github.com/spf13/cobra"
)

func newCRCmd() *cobra.Command {
	crCmd := &cobra.Command{
		Use:     "cr",
		Short:   "Manage change requests",
		Long:    "Change-request commands grouped by intent:\n\nNavigation:\n  current, switch, list, search, stack, status, why\n\nIntake and planning:\n  add, child add, apply, contract set/show, task add, task contract set/show\n\nImplementation lenses:\n  range, rev-parse, pack, diff, task diff, task chunk list/diff, rangediff\n\nValidation and review:\n  impact, validate, review, check run/status\n\nMerge and recovery:\n  merge, merge status, merge resume, merge abort\n\nMetadata and repair:\n  note, evidence (including evidence sample add/list), edit, redact, history, doctor, reconcile, export, import, patch, push, pull, sync, reopen, refresh, base set, restack",
		Example: "  sophia cr add \"Worktree-safe parsing\" --description \"Handle detached worktree edge cases\"\n  sophia cr switch 25\n  sophia cr contract set 25 --why \"Avoid branch context drift\" --scope internal/gitx\n  sophia cr task add 25 \"Add detached-worktree parser test\"\n  sophia cr task done 25 1 --from-contract\n  sophia cr diff 25 --task 1\n  sophia cr review 25\n  sophia cr merge 25",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return rewriteCRSelectorArg(cmd, args)
		},
	}

	crCmd.AddCommand(newCRAddCmd())
	crCmd.AddCommand(newCRApplyCmd())
	crCmd.AddCommand(newCRChildCmd())
	crCmd.AddCommand(newCRListCmd())
	crCmd.AddCommand(newCRSearchCmd())
	crCmd.AddCommand(newCRStackCmd())
	crCmd.AddCommand(newCRRangeCmd())
	crCmd.AddCommand(newCRRevParseCmd())
	crCmd.AddCommand(newCRPackCmd())
	crCmd.AddCommand(newCRArchiveCmd())
	crCmd.AddCommand(newCRDiffCmd())
	crCmd.AddCommand(newCRRangeDiffCmd())
	crCmd.AddCommand(newCRWhyCmd())
	crCmd.AddCommand(newCRStatusCmd())
	crCmd.AddCommand(newCRDoctorCmd())
	crCmd.AddCommand(newCRReconcileCmd())
	crCmd.AddCommand(newCRNoteCmd())
	crCmd.AddCommand(newCREvidenceCmd())
	crCmd.AddCommand(newCRCheckCmd())
	crCmd.AddCommand(newCRReviewCmd())
	crCmd.AddCommand(newCRMergeCmd())
	crCmd.AddCommand(newCRTaskCmd())
	crCmd.AddCommand(newCRCurrentCmd())
	crCmd.AddCommand(newCRSwitchCmd())
	crCmd.AddCommand(newCRReopenCmd())
	crCmd.AddCommand(newCRBaseCmd())
	crCmd.AddCommand(newCRRestackCmd())
	crCmd.AddCommand(newCRRefreshCmd())
	crCmd.AddCommand(newCRBranchCmd())
	crCmd.AddCommand(newCREditCmd())
	crCmd.AddCommand(newCRContractCmd())
	crCmd.AddCommand(newCRImpactCmd())
	crCmd.AddCommand(newCRValidateCmd())
	crCmd.AddCommand(newCRExportCmd())
	crCmd.AddCommand(newCRImportCmd())
	crCmd.AddCommand(newCRPatchCmd())
	crCmd.AddCommand(newCRRedactCmd())
	crCmd.AddCommand(newCRHistoryCmd())
	crCmd.AddCommand(newCRPushCmd())
	crCmd.AddCommand(newCRPullCmd())
	crCmd.AddCommand(newCRSyncCmd())

	return crCmd
}
