package cli

import (
	"sophia/internal/service"

	"github.com/spf13/cobra"
)

func withParsedIDAndService(
	cmd *cobra.Command,
	asJSON bool,
	rawID string,
	argName string,
	run func(id int, svc *service.Service) error,
) error {
	id, svc, err := parseIDAndService(cmd, rawID, argName)
	if err != nil {
		return commandError(cmd, asJSON, err)
	}
	return run(id, svc)
}

func withParsedCRTaskIDsAndService(
	cmd *cobra.Command,
	asJSON bool,
	rawCRID string,
	rawTaskID string,
	run func(crID, taskID int, svc *service.Service) error,
) error {
	crID, taskID, svc, err := parseCRTaskIDsAndService(cmd, rawCRID, rawTaskID)
	if err != nil {
		return commandError(cmd, asJSON, err)
	}
	return run(crID, taskID, svc)
}
