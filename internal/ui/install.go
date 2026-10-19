package ui

import (
	"context"
	"errors"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/basecamp/once/internal/docker"
)

var installKeys = struct {
	Help key.Binding
	Back key.Binding
}{
	Help: WithHelp(NewKeyBinding("f1"), "F1", "help"),
	Back: WithHelp(NewKeyBinding("esc"), "esc", "back"),
}

type installState int

const (
	installStateAppList   installState = iota // Screen 1: choose app
	installStateImageForm                     // Screen 2: enter image ref
	installStateWhere                         // Screen 3: choose where it runs
	installStateHostname                      // Screen 4: enter hostname
	installStateAdvanced                      // Screen 5: advanced settings
	installStateTailscale                     // Screen 6: Tailscale Service options
	installStateActivity                      // Installing
)

type InstallFormSubmitMsg struct {
	ImageRef string
	Hostname string
}

type Install struct {
	namespace          *docker.Namespace
	width, height      int
	help               Help
	state              installState
	appList            InstallAppList
	imageForm          InstallImageForm
	whereForm          InstallWhereForm
	hostnameForm       InstallHostnameForm
	advancedForm       InstallAdvancedForm
	tailscaleForm      InstallTailscaleForm
	activity           *InstallActivity
	popupHelp          *PopupHelp
	starfield          *Starfield
	logo               *Logo
	err                error
	cliMode            bool
	customImage        bool
	customImageRef     string
	installFlag        string
	registry           docker.RegistrySettings
	target             InstallTarget
	pendingImage       string
	pendingHost        string
	pendingSettings    docker.ApplicationSettings
	pendingTailscale   docker.TailscaleSettings
	tailscaleConfirmed bool
}

func NewInstall(ns *docker.Namespace, imageRef string) Install {
	h := NewHelp()
	h.SetBindings([]key.Binding{installKeys.Back})

	m := Install{
		namespace:   ns,
		help:        h,
		cliMode:     imageRef != "",
		installFlag: imageRef,
	}

	if imageRef != "" {
		if expanded, ok := expandAlias(imageRef); ok {
			imageRef = expanded
		}
		m.state = installStateHostname
		m.hostnameForm = NewInstallHostnameForm(imageRef, m.installFlag, InstallTargetPublic)
	} else {
		m.state = installStateAppList
		m.appList = NewInstallAppList()
	}

	if m.showLogo() {
		m.starfield = NewStarfield()
		m.logo = NewLogo()
	}
	return m
}

func (m Install) Init() tea.Cmd {
	cmds := []tea.Cmd{m.initCurrentScreen()}
	if m.showLogo() {
		cmds = append(cmds, m.starfield.Init(), m.logo.Init())
	}
	return tea.Batch(cmds...)
}

func (m Install) Update(msg tea.Msg) (Component, tea.Cmd) {
	if m.popupHelp != nil {
		switch msg := msg.(type) {
		case PopupHelpCloseMsg:
			m.popupHelp = nil
			return m, nil
		case tea.KeyPressMsg, MouseEvent, tea.MouseWheelMsg:
			ph := *m.popupHelp
			var cmd tea.Cmd
			ph, cmd = ph.Update(msg)
			m.popupHelp = &ph
			return m, cmd
		}
	}

	m.updateHelpBindings()

	switch msg := msg.(type) {
	case PopupHelpCloseMsg:
		return m, nil

	case tea.WindowSizeMsg:
		m.popupHelp = nil
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		var cmds []tea.Cmd
		if m.starfield != nil {
			cmds = append(cmds, m.starfield.Update(tea.WindowSizeMsg{Width: m.width, Height: m.middleHeight()}))
		}
		if m.state == installStateActivity {
			cmds = append(cmds, m.activity.Update(msg))
		} else {
			cmds = append(cmds, m.updateCurrentScreen(msg))
		}
		return m, tea.Batch(cmds...)

	case starfieldTickMsg:
		if m.starfield != nil {
			return m, m.starfield.Update(msg)
		}
		return m, nil

	case logoShineStartMsg, logoShineStepMsg:
		if m.logo != nil {
			return m, m.logo.Update(msg)
		}
		return m, nil

	case MouseEvent:
		if m.state != installStateActivity {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			if cmd != nil {
				return m, cmd
			}
		}

	case tea.KeyPressMsg:
		if m.state != installStateActivity {
			if m.err != nil {
				m.err = nil
			}
			if key.Matches(msg, installKeys.Help) {
				if title, content := m.helpForState(); content != "" {
					ph := NewPopupHelp(title, content, m.width, m.height)
					m.popupHelp = &ph
					return m, nil
				}
			}
			if key.Matches(msg, installKeys.Back) {
				return m.handleBack()
			}
		}

	case InstallAppSelectedMsg:
		m.customImage = false
		m.registry = docker.RegistrySettings{}
		m.pendingImage = msg.ImageRef
		m.resetTarget()
		m.state = installStateWhere
		return m, m.initScreenWithSize()

	case InstallCustomSelectedMsg:
		m.imageForm = NewInstallImageForm()
		m.state = installStateImageForm
		return m, m.initScreenWithSize()

	case InstallImageSubmitMsg:
		if msg.RegistryUsername != "" && msg.RegistryPassword == "" {
			m.err = docker.ErrRegistryUsernameWithoutPassword
			return m, nil
		}
		if msg.RegistryPassword != "" && msg.RegistryUsername == "" {
			m.err = docker.ErrRegistryPasswordWithoutUsername
			return m, nil
		}
		m.customImage = true
		m.customImageRef = msg.ImageRef
		m.pendingImage = msg.ImageRef
		m.registry = docker.RegistrySettings{}
		if msg.RegistryUsername != "" || msg.RegistryPassword != "" {
			host, _ := docker.RegistryHost(msg.ImageRef)
			m.registry = docker.RegistrySettings{
				Host:     host,
				Username: msg.RegistryUsername,
				Password: msg.RegistryPassword,
			}
		}
		m.resetTarget()
		m.state = installStateWhere
		return m, m.initScreenWithSize()

	case InstallWhereSelectedMsg:
		m.target = msg.Target
		m.hostnameForm = NewInstallHostnameForm(m.pendingImage, "", m.target)
		m.state = installStateHostname
		return m, m.initScreenWithSize()

	case InstallImageBackMsg:
		m.state = installStateAppList
		return m, nil

	case InstallHostnameBackMsg:
		if m.cliMode {
			return m, m.cancelFromScreen()
		}
		m.state = installStateWhere
		return m, nil

	case InstallFormSubmitMsg:
		if m.namespace.HostInUse(msg.Hostname) {
			m.err = docker.ErrHostnameInUse
			return m, nil
		}
		m.pendingImage = msg.ImageRef
		m.pendingHost = msg.Hostname
		if m.target == InstallTargetTailnet {
			m.tailscaleForm = NewInstallTailscaleForm(m.pendingImage, m.pendingSettings.AppPort, m.pendingTailscale)
			m.state = installStateTailscale
			return m, m.initScreenWithSize()
		}
		return m, m.startActivity(m.pendingSettings)

	case InstallTailscaleSubmitMsg:
		if m.state != installStateTailscale {
			break
		}
		m.pendingTailscale = msg.Tailscale
		m.tailscaleConfirmed = true
		return m, m.startActivity(m.pendingSettings)

	case InstallTailscaleBackMsg:
		m.state = installStateHostname
		return m, nil

	case InstallAdvancedMsg:
		if msg.Hostname == "" {
			m.err = docker.ErrHostRequired
			return m, nil
		}
		if m.namespace.HostInUse(msg.Hostname) {
			m.err = docker.ErrHostnameInUse
			return m, nil
		}
		m.pendingImage = msg.ImageRef
		m.pendingHost = msg.Hostname
		m.advancedForm = NewInstallAdvancedForm(m.pendingSettings)
		m.state = installStateAdvanced
		return m, m.initScreenWithSize()

	case SettingsSectionSubmitMsg:
		if m.state != installStateAdvanced {
			break
		}
		settings := msg.Settings
		settings.Image = m.pendingImage
		settings.Host = m.pendingHost
		if err := settings.Validate(); err != nil {
			m.err = err
			return m, nil
		}
		m.pendingSettings = settings
		if m.target == InstallTargetTailnet && !m.tailscaleConfirmed {
			m.tailscaleForm = NewInstallTailscaleForm(m.pendingImage, m.pendingSettings.AppPort, m.pendingTailscale)
			m.state = installStateTailscale
			return m, m.initScreenWithSize()
		}
		return m, m.startActivity(m.pendingSettings)

	case SettingsSectionCancelMsg:
		if m.state != installStateAdvanced {
			break
		}
		m.state = installStateHostname
		return m, nil

	case InstallActivityFailedMsg:
		_ = m.namespace.Refresh(context.Background())
		m.activity = nil
		m.err = msg.Err
		if errors.Is(msg.Err, docker.ErrPullFailed) {
			m.state = m.imageErrorState()
			if m.state == installStateImageForm && errors.Is(msg.Err, docker.ErrPullMaybeUnauthorized) && !m.imageForm.ShowsCredentials() {
				m.imageForm = NewInstallImageFormWithCredentials(m.customImageRef)
				return m, m.initScreenWithSize()
			}
		} else {
			m.state = installStateHostname
		}

		return m, nil

	case InstallActivityDoneMsg:
		return m, func() tea.Msg { return NavigateToAppMsg(msg) }
	}

	if m.state == installStateActivity {
		return m, m.activity.Update(msg)
	}
	return m, m.updateCurrentScreen(msg)
}

func (m Install) View() string {
	var contentView string
	if m.state == installStateActivity {
		contentView = m.activity.View()
	} else {
		formView := m.viewCurrentScreen()
		if m.err != nil {
			errorLine := lipgloss.NewStyle().Foreground(Colors.Error).Width(m.width).Align(lipgloss.Center).Render(docker.ErrorMessage(m.err))
			formView = lipgloss.JoinVertical(lipgloss.Center, errorLine, "", formView)
		}
		contentView = formView
	}

	var helpLine string
	if m.state != installStateActivity {
		m.updateHelpBindings()
		helpLine = Styles.CenteredLine(m.width, m.help.View())
	}

	middleH := m.middleHeight()

	var result string
	if m.starfield != nil {
		if m.showLogo() && m.state != installStateActivity {
			result = lipgloss.JoinVertical(lipgloss.Left, m.renderLogoWithStarfield(contentView, middleH), helpLine)
		} else {
			result = lipgloss.JoinVertical(lipgloss.Left, m.renderMiddleWithStarfield(contentView, middleH), helpLine)
		}
	} else {
		middle := m.renderMiddleCentered(contentView, middleH)
		titleLine := Styles.TitleRule(m.width, "install")
		result = lipgloss.JoinVertical(lipgloss.Left, titleLine, "", middle, helpLine)
	}

	if m.popupHelp != nil {
		return OverlayCenter(result, m.popupHelp.View(), m.width, m.height)
	}
	return result
}

// Private

func (m Install) initCurrentScreen() tea.Cmd {
	switch m.state {
	case installStateAppList:
		return m.appList.Init()
	case installStateImageForm:
		return m.imageForm.Init()
	case installStateWhere:
		return m.whereForm.Init()
	case installStateHostname:
		return m.hostnameForm.Init()
	case installStateAdvanced:
		return m.advancedForm.Init()
	case installStateTailscale:
		return m.tailscaleForm.Init()
	}
	return nil
}

func (m *Install) initScreenWithSize() tea.Cmd {
	initCmd := m.initCurrentScreen()
	if m.width > 0 {
		m.updateCurrentScreen(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	}
	return initCmd
}

func (m *Install) updateCurrentScreen(msg tea.Msg) tea.Cmd {
	switch m.state {
	case installStateAppList:
		var cmd tea.Cmd
		m.appList, cmd = m.appList.Update(msg)
		return cmd
	case installStateImageForm:
		var cmd tea.Cmd
		m.imageForm, cmd = m.imageForm.Update(msg)
		return cmd
	case installStateWhere:
		var cmd tea.Cmd
		m.whereForm, cmd = m.whereForm.Update(msg)
		return cmd
	case installStateHostname:
		var cmd tea.Cmd
		m.hostnameForm, cmd = m.hostnameForm.Update(msg)
		return cmd
	case installStateAdvanced:
		var cmd tea.Cmd
		m.advancedForm, cmd = m.advancedForm.Update(msg)
		return cmd
	case installStateTailscale:
		var cmd tea.Cmd
		m.tailscaleForm, cmd = m.tailscaleForm.Update(msg)
		return cmd
	}
	return nil
}

func (m Install) viewCurrentScreen() string {
	switch m.state {
	case installStateAppList:
		return m.appList.View()
	case installStateImageForm:
		return m.imageForm.View()
	case installStateWhere:
		return m.whereForm.View()
	case installStateHostname:
		return m.hostnameForm.View()
	case installStateAdvanced:
		return m.advancedForm.View()
	case installStateTailscale:
		return m.tailscaleForm.View()
	}
	return ""
}

func (m Install) handleBack() (Install, tea.Cmd) {
	switch m.state {
	case installStateAppList:
		return m, m.cancelFromScreen()
	case installStateImageForm:
		m.state = installStateAppList
		return m, nil
	case installStateWhere:
		if m.customImage {
			m.state = installStateImageForm
			return m, m.imageForm.Init()
		}
		m.state = installStateAppList
		return m, nil
	case installStateHostname:
		if m.cliMode {
			return m, m.cancelFromScreen()
		}
		m.state = installStateWhere
		return m, nil
	case installStateAdvanced, installStateTailscale:
		m.state = installStateHostname
		return m, nil
	}
	return m, nil
}

// startActivity installs the app: the settings collected so far, the image and
// host, and whether the app runs publicly or on the tailnet.
func (m *Install) startActivity(settings docker.ApplicationSettings) tea.Cmd {
	settings.Image = m.pendingImage
	settings.Host = m.pendingHost
	if m.target == InstallTargetTailnet {
		settings.Tailscale = m.pendingTailscale
	}
	settings = m.target.Apply(settings)

	if err := settings.Validate(); err != nil {
		m.err = err
		return nil
	}

	m.state = installStateActivity
	m.activity = NewInstallActivity(m.namespace, m.pendingImage, m.pendingHost, m.registry, settings)
	m.activity.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return m.activity.Init()
}

// resetTarget returns the app to the default public target, discarding a
// Tailscale Service chosen for a previously selected image.
func (m *Install) resetTarget() {
	m.target = InstallTargetPublic
	m.pendingSettings = docker.ApplicationSettings{}
	m.pendingTailscale = docker.TailscaleSettings{}
	m.tailscaleConfirmed = false
	m.whereForm = NewInstallWhereForm()
}

func (m Install) imageErrorState() installState {
	if m.customImage {
		return installStateImageForm
	}
	return installStateAppList
}

func (m Install) showLogo() bool {
	return len(m.namespace.Applications()) == 0 && os.Getenv("ONCE_REDUCED_MOTION") != "true"
}

func (m Install) middleHeight() int {
	helpHeight := 1
	if m.starfield != nil {
		return max(m.height-helpHeight, 0)
	}
	titleHeight := 2
	return max(m.height-titleHeight-helpHeight, 0)
}

func (m Install) cancelFromScreen() tea.Cmd {
	if m.activity != nil {
		m.activity.Cancel()
	}
	if m.cliMode {
		return func() tea.Msg { return QuitMsg{} }
	}
	return func() tea.Msg { return NavigateToDashboardMsg{} }
}

func (m Install) renderMiddleCentered(contentView string, middleHeight int) string {
	centered := lipgloss.NewStyle().
		Width(m.width).
		Height(middleHeight).
		Align(lipgloss.Center, lipgloss.Center).
		Render(contentView)
	return centered
}

// overlayBlock holds a pre-measured block of lines to composite over the starfield.
type overlayBlock struct {
	lines []string
	width int
	top   int
	left  int
}

func newOverlayBlock(content string, top, left int) overlayBlock {
	lines := strings.Split(content, "\n")
	width := blockWidth(lines)
	return overlayBlock{lines: lines, width: width, top: top, left: left}
}

func (b overlayBlock) containsRow(row int) (line string, ok bool) {
	idx := row - b.top
	if idx < 0 || idx >= len(b.lines) {
		return "", false
	}
	line = b.lines[idx]
	if w := ansi.StringWidth(line); w < b.width {
		line += strings.Repeat(" ", b.width-w)
	}
	return line, true
}

// renderLogoWithStarfield composites the logo (pinned at top) and form (centered)
// as independent layers over the starfield.
func (m Install) renderLogoWithStarfield(formView string, middleHeight int) string {
	m.starfield.ComputeGrid()

	logo := newOverlayBlock(m.logo.View(), 1, 0)
	logo.left = (m.width - logo.width) / 2

	formLines := strings.Split(formView, "\n")
	form := newOverlayBlock(formView, (middleHeight-len(formLines))/2, 0)
	form.left = (m.width - form.width) / 2

	var sb strings.Builder
	for row := range middleHeight {
		if line, ok := form.containsRow(row); ok {
			m.writeOverlayRow(&sb, row, form.left, form.width, line)
		} else if line, ok := logo.containsRow(row); ok {
			m.writeOverlayRow(&sb, row, logo.left, logo.width, line)
		} else {
			sb.WriteString(m.starfield.RenderFullRow(row))
		}

		if row < middleHeight-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// renderMiddleWithStarfield composites the content view over the starfield background.
func (m Install) renderMiddleWithStarfield(contentView string, middleHeight int) string {
	m.starfield.ComputeGrid()

	fgLines := strings.Split(contentView, "\n")
	fg := newOverlayBlock(contentView, (middleHeight-len(fgLines))/2, 0)
	fg.left = (m.width - fg.width) / 2

	var sb strings.Builder
	for row := range middleHeight {
		if line, ok := fg.containsRow(row); ok {
			m.writeOverlayRow(&sb, row, fg.left, fg.width, line)
		} else {
			sb.WriteString(m.starfield.RenderFullRow(row))
		}
		if row < middleHeight-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func (m Install) writeOverlayRow(sb *strings.Builder, row, left, width int, line string) {
	sb.WriteString(m.starfield.RenderRow(row, 0, left))
	sb.WriteString(line)
	sb.WriteString(m.starfield.RenderRow(row, left+width, m.width))
}

func (m *Install) updateHelpBindings() {
	if _, content := m.helpForState(); content != "" {
		m.help.SetBindings([]key.Binding{installKeys.Help, installKeys.Back})
	} else {
		m.help.SetBindings([]key.Binding{installKeys.Back})
	}
}

func (m Install) helpForState() (string, string) {
	switch m.state {
	case installStateHostname:
		if m.target == InstallTargetTailnet {
			return "Naming the Service on your tailnet", installTailnetHelpText
		}
		return "Setting the hostname", installHostnameHelpText
	}
	return "", ""
}

const (
	installHostnameHelpText = `On this screen should enter the hostname where you'll be running this application.

If you're installing an application on the Internet, this should use a domain name that you own. You can use a subdomain so that one domain can support multiple applications.

For example, if you own example.com and are installing Campfire, you might choose chat.example.com as your hostname.

When you use your own domain on a publicly-reachable server, ONCE will set up SSL automatically to secure your application.

If you're installing only on your local machine, you can use a localhost domain instead. You can still use a subdomain to support multiple applications (like chat.localhost). When you install an application in this way, it won't get the automatic SSL.

You can always change these settings later, too: look for them under Settings -> Application.
`

	installTailnetHelpText = `On this screen you should enter the name this application will be known by on your tailnet.

ONCE publishes the app as a Tailscale Service, so the name only needs to be meaningful to you and your tailnet. It doesn't need to be a public domain, and there's no DNS to configure: MagicDNS resolves it for your other devices.

For example, if your tailnet is example.ts.net and you're installing Campfire, you might choose campfire.example.ts.net.

Because traffic stays on the tailnet, ONCE doesn't set up an SSL certificate for this application; Tailscale handles encryption between your devices.

You can always change these settings later, too: look for them under Settings -> Application.
`
)

func blockWidth(lines []string) int {
	width := 0
	for _, line := range lines {
		if w := ansi.StringWidth(line); w > width {
			width = w
		}
	}
	return width
}
