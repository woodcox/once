package command

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"tailscale.com/tsnet"

	"github.com/woodcox/once/internal/docker"
)

const tailscaleServeShutdownTimeout = 10 * time.Second

type tailscaleServeCommand struct {
	cmd *cobra.Command

	hostname string
	port     int
	authKey  string
	stateDir string
}

func newTailscaleServeCommand() *tailscaleServeCommand {
	t := &tailscaleServeCommand{}
	t.cmd = &cobra.Command{
		Use:   "serve HOST",
		Short: "Expose an installed ONCE app on your tailnet using tsnet",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(t.run),
	}

	t.cmd.Flags().StringVar(&t.hostname, "hostname", "", "tailnet hostname for this tsnet service (defaults to app name)")
	t.cmd.Flags().IntVar(&t.port, "port", 443, "tailnet listen port")
	t.cmd.Flags().StringVar(&t.authKey, "auth-key", "", "tailscale auth key for non-interactive login")
	t.cmd.Flags().StringVar(&t.stateDir, "state-dir", "", "directory for tsnet state (defaults to ephemeral state)")

	return t
}

func (t *tailscaleServeCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	app := ns.ApplicationByHost(args[0])
	if app == nil {
		return fmt.Errorf("no application found at host %q", args[0])
	}

	target, err := appProxyURL(ns)
	if err != nil {
		return err
	}

	authKey := t.authKey
	if authKey == "" {
		authKey = app.Settings.Tailscale.AuthKey
	}

	ts := &tsnet.Server{Hostname: t.hostname, AuthKey: authKey, Dir: t.stateDir}
	if ts.Hostname == "" {
		ts.Hostname = "once-" + app.Settings.Name
	}
	defer ts.Close()

	ln, err := ts.Listen("tcp", fmt.Sprintf(":%d", t.port))
	if err != nil {
		return fmt.Errorf("starting tsnet listener: %w", err)
	}
	defer ln.Close()

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = app.Settings.Host
	}
	srv := &http.Server{Handler: proxy}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	fmt.Fprintf(cmd.OutOrStdout(), "Serving %s via tailnet hostname %s on port %d\n", app.Settings.Host, ts.Hostname, t.port)
	fmt.Fprintln(cmd.OutOrStdout(), "Press Ctrl+C to stop")

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("serving tailscale proxy: %w", err)
		}
	case <-sigCh:
	}

	if err := shutdownTailscaleProxy(srv.Shutdown); err != nil {
		return fmt.Errorf("shutting down tailscale proxy: %w", err)
	}
	return nil
}

// Helpers

func shutdownTailscaleProxy(shutdown func(context.Context) error) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), tailscaleServeShutdownTimeout)
	defer cancel()

	return shutdown(shutdownCtx)
}

func appProxyURL(ns *docker.Namespace) (*url.URL, error) {
	proxy := ns.Proxy()
	if proxy.Settings == nil {
		return nil, fmt.Errorf("proxy settings unavailable")
	}
	return &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", proxy.Settings.HTTPPort)}, nil
}
