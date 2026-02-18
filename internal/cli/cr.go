package cli

import (
	"github.com/spf13/cobra"
)

func newCRCmd() *cobra.Command {
	crCmd := &cobra.Command{
		Use:   "cr",
		Short: "Manage change requests",
	}

	crCmd.AddCommand(newCRAddCmd())
	crCmd.AddCommand(newCRApplyCmd())
	crCmd.AddCommand(newCRChildCmd())
	crCmd.AddCommand(newCRListCmd())
	crCmd.AddCommand(newCRSearchCmd())
	crCmd.AddCommand(newCRStackCmd())
	crCmd.AddCommand(newCRDiffCmd())
	crCmd.AddCommand(newCRWhyCmd())
	crCmd.AddCommand(newCRStatusCmd())
	crCmd.AddCommand(newCRDoctorCmd())
	crCmd.AddCommand(newCRReconcileCmd())
	crCmd.AddCommand(newCRNoteCmd())
	crCmd.AddCommand(newCREvidenceCmd())
	crCmd.AddCommand(newCRReviewCmd())
	crCmd.AddCommand(newCRMergeCmd())
	crCmd.AddCommand(newCRTaskCmd())
	crCmd.AddCommand(newCRCurrentCmd())
	crCmd.AddCommand(newCRSwitchCmd())
	crCmd.AddCommand(newCRReopenCmd())
	crCmd.AddCommand(newCRBaseCmd())
	crCmd.AddCommand(newCRRestackCmd())
	crCmd.AddCommand(newCREditCmd())
	crCmd.AddCommand(newCRContractCmd())
	crCmd.AddCommand(newCRImpactCmd())
	crCmd.AddCommand(newCRValidateCmd())
	crCmd.AddCommand(newCRExportCmd())
	crCmd.AddCommand(newCRRedactCmd())
	crCmd.AddCommand(newCRHistoryCmd())

	return crCmd
}
