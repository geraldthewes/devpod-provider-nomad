package cmd

import (
	"context"

	"github.com/briancain/devpod-provider-nomad/pkg/nomad"
	opts "github.com/briancain/devpod-provider-nomad/pkg/options"
	"github.com/loft-sh/log"
	"github.com/spf13/cobra"
)

// DeleteCmd holds the cmd flags
type DeleteCmd struct{}

// NewCommandCmd defines a command
func NewDeleteCmd() *cobra.Command {
	cmd := &DeleteCmd{}
	commandCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a new devpod instance on Nomad",
		RunE: func(_ *cobra.Command, args []string) error {
			options, err := opts.FromEnv()
			if err != nil {
				return err
			}

			return cmd.Run(context.Background(), options)
		},
	}

	return commandCmd
}

func (cmd *DeleteCmd) Run(
	ctx context.Context,
	options *opts.Options,
) error {
	nomadClient, err := nomad.NewNomad(options)
	if err != nil {
		return err
	}

	// First delete the job
	if err := nomadClient.Delete(ctx, options.JobId); err != nil {
		return err
	}

	// If persistent storage mode, also delete the CSI volume
	if options.StorageMode == opts.StorageModePersistent {
		volumeID := options.GetVolumeID()
		logger := log.Default.ErrorStreamOnly()

		// Delete CSI volume - log warning but don't fail if this fails
		// The volume might have already been deleted or might still be detaching
		if err := nomadClient.DeleteCSIVolume(ctx, volumeID, options.Namespace); err != nil {
			logger.Warnf("Failed to delete CSI volume %s: %v (volume may need manual cleanup)", volumeID, err)
		}
	}

	return nil
}
