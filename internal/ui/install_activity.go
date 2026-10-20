package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

type installStage int

const (
	stagePreparing installStage = iota
	stageDownloading
	stageStarting
	stageVerifying
)

const (
	docktailNotAdvertisedPrompt  = "The app is running, but its Tailscale Service hasn't appeared yet. Retry the check, keep the app, or delete it and its data?"
	docktailNotAdvertisedWarning = "the Tailscale Service isn't advertised yet, so the app may not be reachable on your tailnet until docktail reconciles it"
)

// errInstallCancelled reports a cancelled install to the install goroutine, so
// that it can stop without touching the app.
var errInstallCancelled = errors.New("install cancelled")

type installProgressMsg struct {
	stage      installStage
	percentage int
}

// installPromptMsg asks the UI to put up the docktail decision prompt; the
// install goroutine then blocks until the user answers.
type installPromptMsg struct{}

type installDoneMsg struct {
	app *docker.Application
	err error
}

type InstallActivityDoneMsg struct {
	App *docker.Application
}

type InstallActivityFailedMsg struct {
	Err error
}

type installWarningMsg struct {
	warnings []string
}

type InstallActivity struct {
	namespace     *docker.Namespace
	imageRef      string
	hostname      string
	registry      docker.RegistrySettings
	settings      docker.ApplicationSettings
	width, height int
	stage         installStage
	percentage    int
	progress      Progress
	warnings      []string
	prompt        *DocktailPrompt
	progressChan  chan installProgressMsg
	warningChan   chan installWarningMsg
	promptChan    chan installPromptMsg
	decisionChan  chan docktailDecision
	doneChan      chan installDoneMsg
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewInstallActivity(ns *docker.Namespace, imageRef, hostname string, registry docker.RegistrySettings, settings docker.ApplicationSettings) *InstallActivity {
	ctx, cancel := context.WithCancel(context.Background())
	return &InstallActivity{
		namespace:    ns,
		imageRef:     imageRef,
		hostname:     hostname,
		registry:     registry,
		settings:     settings,
		stage:        stagePreparing,
		progress:     NewProgress(0, Colors.Primary),
		progressChan: make(chan installProgressMsg, 10),
		warningChan:  make(chan installWarningMsg, 10),
		promptChan:   make(chan installPromptMsg, 1),
		decisionChan: make(chan docktailDecision, 1),
		doneChan:     make(chan installDoneMsg, 1),
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (m *InstallActivity) Init() tea.Cmd {
	return tea.Batch(m.progress.Init(), m.startInstall(), m.waitForProgress())
}

func (m *InstallActivity) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.progress = m.progress.SetWidth(min(m.width-4, 60))

	case installProgressMsg:
		m.stage = msg.stage
		m.percentage = msg.percentage
		switch msg.stage {
		case stageDownloading:
			m.progress = m.progress.SetPercent(msg.percentage)
		default:
			m.progress = m.progress.SetPercent(-1)
		}
		return m.waitForProgress()

	case installWarningMsg:
		m.warnings = append(m.warnings, msg.warnings...)
		return m.waitForProgress()

	case installPromptMsg:
		prompt := NewDocktailPrompt(docktailNotAdvertisedPrompt)
		m.prompt = &prompt
		return m.waitForProgress()

	case docktailDecisionMsg:
		if m.prompt == nil {
			// A second decision arrived before the first was consumed.
			// Drop it, but keep the progress pump alive.
			return m.waitForProgress()
		}
		m.prompt = nil
		select {
		case m.decisionChan <- msg.decision:
		default:
		}
		return m.waitForProgress()

	case tea.KeyPressMsg, MouseEvent:
		if m.prompt != nil {
			return m.updatePrompt(msg)
		}

	case installDoneMsg:
		if msg.err != nil {
			return func() tea.Msg { return InstallActivityFailedMsg{Err: msg.err} }
		}
		return func() tea.Msg { return InstallActivityDoneMsg{App: msg.app} }

	case ProgressTickMsg:
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return cmd
	}

	return nil
}

func (m *InstallActivity) View() string {
	var status string
	switch m.stage {
	case stagePreparing:
		status = "Preparing..."
	case stageDownloading:
		status = "Downloading..."
	case stageStarting:
		status = "Starting..."
	case stageVerifying:
		status = "Verifying..."
	}

	statusLine := Styles.CenteredLine(m.width, status)

	progressView := Styles.CenteredLine(m.width, m.progress.View())

	parts := []string{statusLine, progressView}

	// Prerequisite problems reported by docktail mode are advisory: the
	// install carries on, but the user should know.
	warningStyle := lipgloss.NewStyle().Foreground(Colors.Warning)
	for _, warning := range m.warnings {
		parts = append(parts, Styles.CenteredLine(m.width, warningStyle.Render(warning)))
	}

	if m.prompt != nil {
		parts = append(parts, "", m.prompt.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *InstallActivity) Cancel() {
	if m.cancel != nil {
		m.cancel()
	}
}

// Private

func (m *InstallActivity) startInstall() tea.Cmd {
	return func() tea.Msg {
		go m.runInstall(m.ctx)
		return nil
	}
}

func (m *InstallActivity) waitForProgress() tea.Cmd {
	return func() tea.Msg {
		// A warning is queued immediately before the terminal message that
		// follows it, so drain warnings first to make sure they're seen.
		select {
		case warning := <-m.warningChan:
			return warning
		default:
		}

		select {
		case progress, ok := <-m.progressChan:
			if ok {
				return progress
			}
		case warning := <-m.warningChan:
			return warning
		case prompt := <-m.promptChan:
			return prompt
		case done := <-m.doneChan:
			return done
		}
		return nil
	}
}

func (m *InstallActivity) updatePrompt(msg tea.Msg) tea.Cmd {
	if m.prompt == nil {
		return nil
	}

	prompt, cmd := m.prompt.Update(msg)
	m.prompt = &prompt
	return cmd
}

func (m *InstallActivity) runInstall(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			m.doneChan <- installDoneMsg{err: fmt.Errorf("install panicked: %v", r)}
		}
	}()

	m.progressChan <- installProgressMsg{stage: stagePreparing}

	if err := m.namespace.Setup(ctx); err != nil {
		m.doneChan <- installDoneMsg{err: fmt.Errorf("%w: %w", docker.ErrSetupFailed, err)}
		return
	}

	m.progressChan <- installProgressMsg{stage: stageDownloading, percentage: 0}

	appName, err := m.namespace.UniqueName(docker.NameFromImageRef(m.imageRef))
	if err != nil {
		m.doneChan <- installDoneMsg{err: fmt.Errorf("generating app name: %w", err)}
		return
	}
	hostname := m.hostname

	settings := m.settings
	settings.Name = appName
	settings.Image = m.imageRef
	settings.Host = hostname
	settings.AutoUpdate = true
	settings.Registry = m.registry

	app := docker.NewApplication(m.namespace, settings)

	for _, warning := range app.DocktailWarnings(ctx) {
		slog.Warn("Docktail prerequisite", "app", appName, "warning", warning)
		m.warningChan <- installWarningMsg{warnings: []string{warning}}
	}

	progress := func(p docker.DeployProgress) {
		switch p.Stage {
		case docker.DeployStageDownloading:
			m.progressChan <- installProgressMsg{stage: stageDownloading, percentage: p.Percentage}
		case docker.DeployStageStarting:
			m.progressChan <- installProgressMsg{stage: stageStarting, percentage: 100}
		}
	}

	if err := app.Deploy(ctx, progress); err != nil {
		if cleanupErr := app.Destroy(context.Background(), true); cleanupErr != nil {
			slog.Error("Failed to clean up after deploy failure", "app", appName, "error", cleanupErr)
		}
		m.doneChan <- installDoneMsg{err: fmt.Errorf("%w: %w", docker.ErrDeployFailed, err)}
		return
	}

	m.progressChan <- installProgressMsg{stage: stageVerifying}

	err = m.verifyWithPrompt(ctx, app)
	if errors.Is(err, errInstallCancelled) {
		return
	}
	if err != nil {
		m.doneChan <- installDoneMsg{err: err}
		return
	}

	m.doneChan <- installDoneMsg{app: app}
}

// verifyWithPrompt checks the deployed app, asking the user what to do when a
// Tailscale Service hasn't been advertised within the verification window. It
// returns nil to keep the app running, the verification error to fail the
// install, or errInstallCancelled when the install has been cancelled and the
// app must be left alone.
func (m *InstallActivity) verifyWithPrompt(ctx context.Context, app *docker.Application) error {
	for {
		err := app.Verify(ctx)
		if err == nil {
			return nil
		}

		if !errors.Is(err, docker.ErrDocktailNotAdvertised) {
			m.removeAfterFailedVerification(app)
			return err
		}

		decision, ok := m.promptForDecision(ctx)
		if !ok {
			return errInstallCancelled
		}

		switch decision {
		case docktailRetry:
			// loop and verify again with a fresh window
		case docktailKeep:
			m.warningChan <- installWarningMsg{warnings: []string{docktailNotAdvertisedWarning}}
			return nil
		case docktailRollback:
			m.removeAfterFailedVerification(app)
			return err
		}
	}
}

// promptForDecision asks the UI to show the prompt and blocks until the user
// answers. It reports false if the install is cancelled while waiting, so that
// a cancelled install neither hangs nor rolls the app back.
func (m *InstallActivity) promptForDecision(ctx context.Context) (docktailDecision, bool) {
	m.promptChan <- installPromptMsg{}

	select {
	case decision := <-m.decisionChan:
		return decision, true
	case <-ctx.Done():
		return 0, false
	}
}

func (m *InstallActivity) removeAfterFailedVerification(app *docker.Application) {
	if err := app.Remove(context.Background(), true); err != nil {
		slog.Error("Failed to clean up after verification failure", "app", app.Settings.Name, "error", err)
	}
}
