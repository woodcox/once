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
	docktailLabelEnable      = "docktail.service.enable"
	docktailLabelName        = "docktail.service.name"
	docktailLabelPort        = "docktail.service.port"
	docktailLabelServicePort = "docktail.service.service-port"
	docktailLabelPath        = "docktail.service.path"
	docktailLabelTags        = "docktail.tags"
)

// TailscaleSettings configures exposure of an application as a native
// Tailscale Service. Only docktail label mode is supported: ONCE sets
// `docktail.*` labels on the app container and the host's docktail container
// advertises it to tailscaled.
type TailscaleSettings struct {
	Enabled     bool   `json:"enabled,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Service     string `json:"service,omitempty"`
	Port        int    `json:"port,omitempty"`
	ServicePort int    `json:"servicePort,omitempty"`
	Path        string `json:"path,omitempty"`
	Tags        string `json:"tags,omitempty"`
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
	Tailscale       TailscaleSettings  `json:"tailscale,omitempty"`
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

// DocktailLabels returns the `docktail.*` labels to place on the app
// container, or nil when docktail mode is off. Optional labels are omitted so
// that docktail's own defaults apply.
func (s ApplicationSettings) DocktailLabels() map[string]string {
	if !s.DocktailEnabled() {
		return nil
	}

	labels := map[string]string{
		docktailLabelEnable: "true",
		docktailLabelName:   s.EffectiveServiceName(),
		docktailLabelPort:   strconv.Itoa(s.EffectiveTailscalePort()),
	}

	if s.Tailscale.ServicePort != 0 && s.Tailscale.ServicePort != 80 {
		labels[docktailLabelServicePort] = strconv.Itoa(s.Tailscale.ServicePort)
	}
	if s.Tailscale.Path != "" && s.Tailscale.Path != "/" {
		labels[docktailLabelPath] = s.Tailscale.Path
	}
	if s.Tailscale.Tags != "" {
		labels[docktailLabelTags] = s.Tailscale.Tags
	}

	return labels
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
	if s.Tailscale != other.Tailscale {
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

// Helpers

// SanitizeDNSLabel reduces an arbitrary string to a lowercase DNS label
// suitable for use as a Tailscale Service name. Runs of invalid characters
// collapse to a single hyphen, and leading/trailing hyphens are dropped.
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

	return b.String()
}

func ParseVolumePaths(s string) []string {
	var paths []string
	for _, p := range strings.Split(s, ",") {
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
