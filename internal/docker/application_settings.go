package docker

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type Keys struct {
	SecretKeyBase   string `json:"secretKeyBase,omitempty"`
	VAPIDPublicKey  string `json:"vapidPublicKey,omitempty"`
	VAPIDPrivateKey string `json:"vapidPrivateKey,omitempty"`
}

func (k Keys) Empty() bool {
	return k == Keys{}
}

func (k *Keys) SetVAPIDKey(privateKey string) error {
	key, err := parseVAPIDPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("invalid VAPID private key: %w", err)
	}

	k.VAPIDPublicKey = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	k.VAPIDPrivateKey = base64.RawURLEncoding.EncodeToString(key.Bytes())

	return nil
}

func (k *Keys) Regenerate(secretKeyBase, vapid bool) error {
	if secretKeyBase {
		skb, err := generateSecretKeyBase()
		if err != nil {
			return err
		}
		k.SecretKeyBase = skb
	}

	if vapid {
		pub, priv, err := generateVAPIDKeyPair()
		if err != nil {
			return err
		}
		k.VAPIDPublicKey = pub
		k.VAPIDPrivateKey = priv
	}

	return nil
}

type SMTPSettings struct {
	Server   string `json:"server,omitempty"`
	Port     string `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from,omitempty"`
}

func (s SMTPSettings) BuildEnv() []string {
	if s.Server == "" {
		return nil
	}
	return []string{
		"SMTP_ADDRESS=" + s.Server,
		"SMTP_PORT=" + s.Port,
		"SMTP_USERNAME=" + s.Username,
		"SMTP_PASSWORD=" + s.Password,
		"MAILER_FROM_ADDRESS=" + s.From,
	}
}

type RegistrySettings struct {
	Host     string `json:"host,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func (r RegistrySettings) Empty() bool {
	return r == RegistrySettings{}
}

type ContainerResources struct {
	CPUs     int `json:"cpus,omitempty"`
	MemoryMB int `json:"memoryMB,omitempty"`
}

type BackupSettings struct {
	Path       string `json:"path,omitempty"`
	AutoBackup bool   `json:"autoBackup,omitempty"`
}

// DocktailMode is the only supported value for TailscaleSettings.Mode. An
// empty mode means the feature is off.
const DocktailMode = "docktail"

// Docktail label keys, as understood by https://docktail.org.
const (
	docktailLabelEnable          = "docktail.service.enable"
	docktailLabelName            = "docktail.service.name"
	docktailLabelPort            = "docktail.service.port"
	docktailLabelServicePort     = "docktail.service.service-port"
	docktailLabelServiceProtocol = "docktail.service.service-protocol"
	docktailLabelPath            = "docktail.service.path"
	docktailLabelProtocol        = "docktail.service.protocol"
	docktailLabelProxyProtocol   = "docktail.service.proxy-protocol"
	docktailLabelDescription     = "docktail.service.description"
	docktailLabelDirect          = "docktail.service.direct"
	docktailLabelNetwork         = "docktail.service.network"
	docktailLabelTags            = "docktail.tags"
	docktailLabelFunnelEnable    = "docktail.funnel.enable"
)

// defaultDocktailTags is the value used for `docktail.tags` when the user
// hasn't provided one. docktail's own default (its DEFAULT_SERVICE_TAGS env)
// is "tag:container"; we set it explicitly so an image-supplied tag is
// overridden.
const defaultDocktailTags = "tag:container"

// TailscaleSettings configures exposure of an application as a native
// Tailscale Service. Only docktail label mode is supported: ONCE sets
// `docktail.*` labels on the app container and the host's docktail container
// advertises it to tailscaled.
type TailscaleSettings struct {
	Enabled         bool   `json:"enabled,omitempty"`
	Mode            string `json:"mode,omitempty"`
	Service         string `json:"service,omitempty"`
	Port            int    `json:"port,omitempty"`
	ServicePort     int    `json:"servicePort,omitempty"`
	Path            string `json:"path,omitempty"`
	Tags            string `json:"tags,omitempty"`
	Description     string `json:"description,omitempty"`
	Protocol        string `json:"protocol,omitempty"`
	ServiceProtocol string `json:"serviceProtocol,omitempty"`
	ProxyProtocol   string `json:"proxyProtocol,omitempty"`
	Direct          *bool  `json:"direct,omitempty"`
	Network         string `json:"network,omitempty"`
}

func (t TailscaleSettings) Equal(other TailscaleSettings) bool {
	if !equalDirect(t.Direct, other.Direct) {
		return false
	}
	t.Direct, other.Direct = nil, nil
	return t == other
}

// equalDirect treats a nil Direct and an explicit true as the same: docktail's
// own default is true, so a settings value set by the CLI never needs to be
// distinguished from one left unset in the UI.
func equalDirect(a, b *bool) bool {
	if a == nil || b == nil {
		return (a == nil || *a) && (b == nil || *b)
	}
	return *a == *b
}

type ApplicationSettings struct {
	Name            string             `json:"name"`
	Image           string             `json:"image"`
	Host            string             `json:"host"`
	DisableTLS      bool               `json:"disableTLS"`
	HealthCheckPath string             `json:"healthCheckPath,omitempty"`
	AppPort         int                `json:"appPort,omitempty"`
	VolumePaths     []string           `json:"volumePaths,omitempty"`
	SkipRailsEnv    bool               `json:"skipRailsEnv,omitempty"`
	Tailscale       TailscaleSettings  `json:"tailscale"`
	EnvVars         map[string]string  `json:"env"`
	SMTP            SMTPSettings       `json:"smtp"`
	Resources       ContainerResources `json:"resources"`
	AutoUpdate      bool               `json:"autoUpdate"`
	Backup          BackupSettings     `json:"backup"`
	Keys            Keys               `json:"keys"`
	Registry        RegistrySettings   `json:"registry"`
}

func UnmarshalApplicationSettings(s string) (ApplicationSettings, error) {
	var settings ApplicationSettings
	err := json.Unmarshal([]byte(s), &settings)
	return settings, err
}

func (s ApplicationSettings) Marshal() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (s ApplicationSettings) Validate() error {
	if s.Image == "" {
		return ErrImageRequired
	}
	if s.AppPort < 0 || s.AppPort > 65535 {
		return ErrInvalidAppPort
	}
	if s.HealthCheckPath != "" && !strings.HasPrefix(s.HealthCheckPath, "/") {
		return ErrInvalidHealthCheckPath
	}
	for _, path := range s.VolumePaths {
		if !strings.HasPrefix(path, "/") {
			return ErrInvalidVolumePath
		}
	}
	if s.Backup.AutoBackup && s.Backup.Path == "" {
		return ErrAutoBackupWithoutPath
	}
	if s.Registry.Password != "" && s.Registry.Username == "" {
		return ErrRegistryPasswordWithoutUsername
	}
	if s.Registry.Username != "" && s.Registry.Password == "" {
		return ErrRegistryUsernameWithoutPassword
	}
	return s.ValidateTailscale()
}

// ValidateTailscale checks the docktail settings on their own, so that they
// can be validated before the rest of the application is known (during
// install, for example, where the image and generated name aren't set yet).
func (s ApplicationSettings) ValidateTailscale() error {
	if s.Tailscale.Port < 0 || s.Tailscale.Port > 65535 {
		return ErrInvalidDocktailPort
	}
	if s.Tailscale.ServicePort < 0 || s.Tailscale.ServicePort > 65535 {
		return ErrInvalidDocktailPort
	}
	if s.Tailscale.Path != "" && !strings.HasPrefix(s.Tailscale.Path, "/") {
		return ErrInvalidDocktailPath
	}
	switch s.Tailscale.Protocol {
	case "", "http", "https", "https+insecure", "tcp", "tls-terminated-tcp":
	default:
		return ErrInvalidDocktailProtocol
	}
	switch s.Tailscale.ServiceProtocol {
	case "", "http", "https", "tcp", "tls-terminated-tcp":
	default:
		return ErrInvalidDocktailServiceProtocol
	}
	switch s.Tailscale.ProxyProtocol {
	case "", "1", "2":
	default:
		return ErrInvalidDocktailProxyProtocol
	}

	serviceProtocol := s.effectiveServiceProtocol()
	if s.Tailscale.ProxyProtocol != "" && !isTCPProtocol(serviceProtocol) {
		return ErrDocktailProxyProtocolWithoutTCP
	}
	if s.Tailscale.Path != "" && s.Tailscale.Path != "/" && isTCPProtocol(serviceProtocol) {
		return ErrDocktailPathOnTCP
	}

	if !s.Tailscale.Enabled {
		return nil
	}
	if s.Tailscale.Mode != DocktailMode {
		return ErrInvalidDocktailMode
	}
	if s.EffectiveServiceName() == "" && (s.Tailscale.Service != "" || s.Name != "") {
		return ErrDocktailServiceRequired
	}
	return nil
}

func (s ApplicationSettings) TLSEnabled() bool {
	return s.Host != "" && !s.DisableTLS && !IsLocalhost(s.Host)
}

// EffectiveHealthCheckPath returns the path used to verify the application is
// serving, falling back to the conventional health check path.
func (s ApplicationSettings) EffectiveHealthCheckPath() string {
	if s.HealthCheckPath != "" {
		return s.HealthCheckPath
	}
	return HealthCheckPath
}

// EffectiveAppPort returns the port the application listens on inside its
// container, falling back to the conventional application port.
func (s ApplicationSettings) EffectiveAppPort() int {
	if s.AppPort != 0 {
		return s.AppPort
	}
	return DefaultAppPort
}

// DeployTarget returns the value to register with kamal-proxy for the given
// container: the container id alone when the app uses the default port, or a
// container id and port pair otherwise.
func (s ApplicationSettings) DeployTarget(containerID string) string {
	if s.AppPort != 0 {
		return containerID + ":" + strconv.Itoa(s.AppPort)
	}
	return containerID
}

// EffectiveVolumePaths returns the in-container paths where the application's
// data volume is mounted.
func (s ApplicationSettings) EffectiveVolumePaths() []string {
	if len(s.VolumePaths) > 0 {
		return s.VolumePaths
	}
	return AppVolumeMountTargets
}

func (s ApplicationSettings) VolumePathsString() string {
	return strings.Join(s.EffectiveVolumePaths(), ", ")
}

// DocktailEnabled reports whether this app should be advertised as a native
// Tailscale Service via docktail labels.
func (s ApplicationSettings) DocktailEnabled() bool {
	return s.Tailscale.Enabled && s.Tailscale.Mode == DocktailMode
}

// EffectiveServiceName returns the Tailscale Service name, defaulting to the
// application name, reduced to a tailnet-legal DNS label.
func (s ApplicationSettings) EffectiveServiceName() string {
	if s.Tailscale.Service != "" {
		return SanitizeDNSLabel(s.Tailscale.Service)
	}
	return SanitizeDNSLabel(s.Name)
}

// EffectiveTailscalePort returns the container port docktail should proxy to,
// defaulting to the application's own port.
func (s ApplicationSettings) EffectiveTailscalePort() int {
	if s.Tailscale.Port != 0 {
		return s.Tailscale.Port
	}
	return s.EffectiveAppPort()
}

// EffectiveServicePort returns the port Tailscale should listen on, defaulting
// to 443 when the effective service protocol is https and 80 otherwise.
func (s ApplicationSettings) EffectiveServicePort() int {
	if s.Tailscale.ServicePort != 0 {
		return s.Tailscale.ServicePort
	}
	if s.effectiveServiceProtocol() == "https" {
		return 443
	}
	return 80
}

// EffectiveProtocol returns the backend protocol docktail will use, defaulting
// to https on the backend port 443 and http otherwise. Exposed so the install
// form can show the same preview the docker package will end up emitting.
func (s ApplicationSettings) EffectiveProtocol() string {
	return s.effectiveProtocol()
}

// EffectiveServiceProtocol returns the Tailscale-facing protocol docktail
// will use. Exposed for the same reason as EffectiveProtocol.
func (s ApplicationSettings) EffectiveServiceProtocol() string {
	return s.effectiveServiceProtocol()
}

// DocktailLabels returns the `docktail.*` labels to place on the app container.
// Every key ONCE can determine is set explicitly, including the funnel enable
// and service enable flags, so a Docker image can't smuggle its own labels
// into docktail's reads. Description, network and proxy-protocol remain
// user-set-only and are emitted only when the user supplied a value; any
// image-supplied value for those keys survives, which is documented in the
// README label table.
func (s ApplicationSettings) DocktailLabels() map[string]string {
	labels := map[string]string{
		docktailLabelFunnelEnable: "false",
	}

	if !s.DocktailEnabled() {
		labels[docktailLabelEnable] = "false"
		return labels
	}

	labels[docktailLabelEnable] = "true"
	labels[docktailLabelName] = s.EffectiveServiceName()
	labels[docktailLabelPort] = strconv.Itoa(s.EffectiveTailscalePort())
	labels[docktailLabelServicePort] = strconv.Itoa(s.EffectiveServicePort())
	labels[docktailLabelProtocol] = s.effectiveProtocol()
	labels[docktailLabelServiceProtocol] = s.effectiveServiceProtocol()

	labels[docktailLabelDirect] = strconv.FormatBool(s.effectiveDirect())

	tags := s.Tailscale.Tags
	if tags == "" {
		tags = defaultDocktailTags
	}
	labels[docktailLabelTags] = tags

	if s.Tailscale.Description != "" {
		labels[docktailLabelDescription] = s.Tailscale.Description
	}
	if s.Tailscale.Network != "" {
		labels[docktailLabelNetwork] = s.Tailscale.Network
	}
	if s.Tailscale.ProxyProtocol != "" {
		labels[docktailLabelProxyProtocol] = s.Tailscale.ProxyProtocol
	}

	// docktail rejects a path for TCP service protocols; emit only when
	// the effective service protocol is not TCP, mirroring that rule.
	if !isTCPProtocol(s.effectiveServiceProtocol()) && s.Tailscale.Path != "" && s.Tailscale.Path != "/" {
		labels[docktailLabelPath] = s.Tailscale.Path
	}

	return labels
}

// effectiveDirect returns the value to advertise for `docktail.service.direct`,
// treating an unset Direct as true (which matches docktail's own default).
func (s ApplicationSettings) effectiveDirect() bool {
	if s.Tailscale.Direct == nil {
		return true
	}
	return *s.Tailscale.Direct
}

func (s ApplicationSettings) Equal(other ApplicationSettings) bool {
	if s.Name != other.Name || s.Image != other.Image || s.Host != other.Host || s.DisableTLS != other.DisableTLS {
		return false
	}
	if s.HealthCheckPath != other.HealthCheckPath || s.AppPort != other.AppPort || s.SkipRailsEnv != other.SkipRailsEnv {
		return false
	}
	if !slices.Equal(s.VolumePaths, other.VolumePaths) {
		return false
	}
	if !s.Tailscale.Equal(other.Tailscale) {
		return false
	}
	if s.Resources != other.Resources {
		return false
	}
	if s.SMTP != other.SMTP {
		return false
	}
	if s.AutoUpdate != other.AutoUpdate {
		return false
	}
	if s.Backup != other.Backup {
		return false
	}
	if s.Keys != other.Keys {
		return false
	}
	if s.Registry != other.Registry {
		return false
	}
	if len(s.EnvVars) != len(other.EnvVars) {
		return false
	}
	for k, v := range s.EnvVars {
		if other.EnvVars[k] != v {
			return false
		}
	}
	return true
}

// Private

// effectiveProtocol returns the backend protocol docktail will use, defaulting
// to https on the backend port 443 and http otherwise.
func (s ApplicationSettings) effectiveProtocol() string {
	if s.Tailscale.Protocol != "" {
		return s.Tailscale.Protocol
	}
	if s.EffectiveTailscalePort() == 443 {
		return "https"
	}
	return "http"
}

// effectiveServiceProtocol returns the Tailscale-facing protocol docktail will
// use, mirroring docktail's smart defaults: explicit value, else (when
// ServicePort is set) https on 443, tcp for a TCP backend, else http; else
// https for an http/https/https+insecure backend, tcp for a tcp/tls-terminated-tcp
// backend, http otherwise.
func (s ApplicationSettings) effectiveServiceProtocol() string {
	if s.Tailscale.ServiceProtocol != "" {
		return s.Tailscale.ServiceProtocol
	}
	if s.Tailscale.ServicePort != 0 {
		if s.Tailscale.ServicePort == 443 {
			return "https"
		}
		if isTCPProtocol(s.effectiveProtocol()) {
			return "tcp"
		}
		return "http"
	}
	switch s.effectiveProtocol() {
	case "tcp", "tls-terminated-tcp":
		return "tcp"
	default:
		// http, https, https+insecure, or anything else → https on the
		// Tailscale side, since there's no way to ask for plain http from
		// Tailscale without an explicit service protocol.
		return "https"
	}
}

// Helpers

func isTCPProtocol(protocol string) bool {
	return protocol == "tcp" || protocol == "tls-terminated-tcp"
}

const maxDNSLabelLength = 63

// SanitizeDNSLabel reduces an arbitrary string to a lowercase DNS label
// suitable for use as a Tailscale Service name. Runs of invalid characters
// collapse to a single hyphen, leading/trailing hyphens are dropped, and the
// result is capped at the 63-character DNS label limit.
func SanitizeDNSLabel(s string) string {
	var b strings.Builder
	pendingHyphen := false

	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingHyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingHyphen = false
			b.WriteRune(r)
			continue
		}
		pendingHyphen = true
	}

	label := b.String()
	if len(label) > maxDNSLabelLength {
		label = strings.TrimSuffix(label[:maxDNSLabelLength], "-")
	}

	return label
}

func ParseVolumePaths(s string) []string {
	var paths []string
	for p := range strings.SplitSeq(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

func generateSecretKeyBase() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateVAPIDKeyPair() (publicKey, privateKey string, err error) {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}

	privateKey = base64.RawURLEncoding.EncodeToString(key.Bytes())
	publicKey = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())

	return publicKey, privateKey, nil
}

// parseVAPIDPrivateKey accepts any common base64 dialect (url-safe or
// standard, padded or not) of a raw P-256 private key.
func parseVAPIDPrivateKey(s string) (*ecdh.PrivateKey, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("+", "-", "/", "_").Replace(s)
	s = strings.TrimRight(s, "=")

	bytes, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}

	return ecdh.P256().NewPrivateKey(bytes)
}
