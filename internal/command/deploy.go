package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

// errDocktailDecisionDeferred reports that the deploy itself succeeded, but the
// user has to decide what to do about a Tailscale Service that hasn't been
// advertised. The question is asked once the progress UI has exited, because
// answering it needs to read from the terminal.
var errDocktailDecisionDeferred = errors.New("docktail decision deferred")

// errDocktailDelete reports that the user chose to delete an app whose Service
// never appeared. It is not a failure: the caller rolls the app back and
// reports success.
var errDocktailDelete = errors.New("docktail deploy rolled back")

type deployCommand struct {
	cmd   *cobra.Command
	flags settingsFlags
}

func newDeployCommand() *deployCommand {
	d := &deployCommand{}
	d.cmd = &cobra.Command{
		Use:   "deploy <image>",
		Short: "Deploy an application",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(d.run),
	}

	d.flags.register(d.cmd)

	return d
}

// Private

func (d *deployCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	imageRef := args[0]

	if err := ns.Setup(ctx); err != nil {
		return fmt.Errorf("%w: %w", docker.ErrSetupFailed, err)
	}

	host := d.flags.host
	if host == "" {
		host = docker.NameFromImageRef(imageRef) + ".localhost"
	}

	if ns.HostInUse(host) {
		return docker.ErrHostnameInUse
	}

	settings, err := d.flags.buildSettings(cmd, imageRef, host)
	if err != nil {
		return err
	}

	baseName := docker.NameFromImageRef(imageRef)
	name, err := ns.UniqueName(baseName)
	if err != nil {
		return fmt.Errorf("generating app name: %w", err)
	}
	settings.Name = name

	app := docker.NewApplication(ns, settings)

	for _, warning := range app.DocktailWarnings(ctx) {
		slog.Warn("Docktail prerequisite", "app", name, "warning", warning)
	}

	err = runWithProgress("Deploying "+host, func(progress docker.DeployProgressCallback) error {
		if err := app.Deploy(ctx, progress); err != nil {
			if cleanupErr := app.Destroy(context.Background(), true); cleanupErr != nil {
				slog.Error("Failed to clean up after deploy failure", "app", name, "error", cleanupErr)
			}
			return fmt.Errorf("%w: %w", docker.ErrDeployFailed, err)
		}

		if err := app.Verify(ctx); err != nil {
			// Only a Service that hasn't been advertised yet is worth
			// asking about; anything else fails as it always has.
			if errors.Is(err, docker.ErrDocktailNotAdvertised) {
				return errDocktailDecisionDeferred
			}
			d.removeAfterVerificationFailure(app)
			return err
		}

		return nil
	})

	if errors.Is(err, errDocktailDecisionDeferred) {
		return d.resolveDocktailNotAdvertised(ctx, app, cmd)
	}

	return err
}

func (d *deployCommand) resolveDocktailNotAdvertised(ctx context.Context, app *docker.Application, cmd *cobra.Command) error {
	out := cmd.OutOrStdout()

	if !interactive(cmd) {
		fmt.Fprintln(out, docker.ErrorMessage(docker.ErrDocktailNotAdvertised))
		fmt.Fprintln(out, "Keeping the app.")
		return nil
	}

	if err := decideDocktailNotAdvertised(ctx, app.Verify, cmd.InOrStdin(), out); err != nil {
		if errors.Is(err, errDocktailDelete) {
			d.removeAfterVerificationFailure(app)
			return nil
		}
		return err
	}

	return nil
}

func (d *deployCommand) removeAfterVerificationFailure(app *docker.Application) {
	if err := app.Remove(context.Background(), true); err != nil {
		slog.Error("Failed to clean up after verification failure", "app", app.Settings.Name, "error", err)
	}
}

// decideDocktailNotAdvertised asks how to handle an app whose Tailscale Service
// hasn't appeared, re-running verification when the user asks for a retry. It
// returns nil when the app should be kept, docker.ErrDocktailNotAdvertised when
// it should be rolled back, and any other verification error as it is. Input
// that can't be understood -- including end of input -- keeps the app, so an
// unattended deploy is never deleted by a question it can't answer.
func decideDocktailNotAdvertised(ctx context.Context, verify func(context.Context) error, in io.Reader, out io.Writer) error {
	fmt.Fprintln(out, docker.ErrorMessage(docker.ErrDocktailNotAdvertised))

	reader := bufio.NewReader(in)

	for {
		fmt.Fprint(out, "Retry the check, keep the app, or delete it and its data? [r/k/d] ")

		line, _ := reader.ReadString('\n')

		switch strings.ToLower(strings.TrimSpace(line)) {
		case "r", "retry":
			if err := verify(ctx); err != nil {
				if errors.Is(err, docker.ErrDocktailNotAdvertised) {
					continue
				}
				return err
			}
			fmt.Fprintln(out, "The Tailscale Service is now advertised.")
			return nil
		case "d", "delete":
			return errDocktailDelete
		default:
			fmt.Fprintln(out, "Keeping the app.")
			return nil
		}
	}
}

// Helpers

func interactive(cmd *cobra.Command) bool {
	return streamIsTerminal(cmd.InOrStdin()) && streamIsTerminal(cmd.OutOrStdout())
}

func streamIsTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	return ok && term.IsTerminal(file.Fd())
}
