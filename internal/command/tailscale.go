package command

import "github.com/spf13/cobra"

type tailscaleCommand struct {
	cmd *cobra.Command
}

// newTailscaleCommand creates and initializes a tailscaleCommand with a Cobra root
// command named "tailscale" and registers the serve subcommand.
func newTailscaleCommand() *tailscaleCommand {
	t := &tailscaleCommand{}
	t.cmd = &cobra.Command{
		Use:   "tailscale",
		Short: "Run ONCE services on your tailnet",
	}

	t.cmd.AddCommand(newTailscaleServeCommand().cmd)
	return t
}