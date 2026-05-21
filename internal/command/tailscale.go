package command

import "github.com/spf13/cobra"

type tailscaleCommand struct {
	cmd *cobra.Command
}

func newTailscaleCommand() *tailscaleCommand {
	t := &tailscaleCommand{}
	t.cmd = &cobra.Command{
		Use:   "tailscale",
		Short: "Run ONCE services on your tailnet",
	}

	t.cmd.AddCommand(newTailscaleServeCommand().cmd)
	return t
}