package docker

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

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

type ContainerResources struct {
	CPUs     int `json:"cpus,omitempty"`
	MemoryMB int `json:"memoryMB,omitempty"`
}

type BackupSettings struct {
	Path       string `json:"path,omitempty"`
	AutoBackup bool   `json:"autoBackup,omitempty"`
}

type TailscaleSettings struct {
	Enabled bool   `json:"enabled,omitempty"`
	AuthKey string `json:"authKey,omitempty"`
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
	EnvVars         map[string]string  `json:"env"`
	SMTP            SMTPSettings       `json:"smtp"`
	Resources       ContainerResources `json:"resources"`
	AutoUpdate      bool               `json:"autoUpdate"`
	Backup          BackupSettings     `json:"backup"`
	Tailscale       TailscaleSettings  `json:"tailscale,omitempty"`
}

// It returns the parsed ApplicationSettings and any error encountered during JSON unmarshalling.
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
	if s.Backup.AutoBackup && s.Backup.Path == "" {
		return ErrAutoBackupWithoutPath
	}
	return nil
}

func (s ApplicationSettings) TLSEnabled() bool {
	return s.Host != "" && !s.DisableTLS && !IsLocalhost(s.Host)
}

func (s ApplicationSettings) EffectiveHealthCheckPath() string {
	if s.HealthCheckPath != "" {
		return s.HealthCheckPath
	}
	return DefaultHealthCheckPath
}

func (s ApplicationSettings) DeployTarget(containerID string) string {
	if s.AppPort != 0 {
		return containerID + ":" + strconv.Itoa(s.AppPort)
	}
	return containerID
}

func (s ApplicationSettings) EffectiveVolumePaths() []string {
	if len(s.VolumePaths) > 0 {
		return s.VolumePaths
	}
	return DefaultVolumePaths
}

func (s ApplicationSettings) VolumePathsString() string {
	return strings.Join(s.EffectiveVolumePaths(), ", ")
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
	if s.Tailscale != other.Tailscale {
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

func (s ApplicationSettings) BuildEnv(vol ApplicationVolumeSettings) []string {
	var env []string

	if !s.SkipRailsEnv {
		env = append(env,
			"SECRET_KEY_BASE="+vol.SecretKeyBase,
			"VAPID_PUBLIC_KEY="+vol.VAPIDPublicKey,
			"VAPID_PRIVATE_KEY="+vol.VAPIDPrivateKey,
		)
		if !s.TLSEnabled() {
			env = append(env, "DISABLE_SSL=true")
		}
	}

	if s.Resources.CPUs > 0 {
		env = append(env, "NUM_CPUS="+strconv.Itoa(s.Resources.CPUs))
	}

	env = append(env, s.SMTP.BuildEnv()...)

	for k, v := range s.EnvVars {
		env = append(env, k+"="+v)
	}

	return env
}

// ParseVolumePaths splits s on commas, trims whitespace from each segment, and returns a slice of non-empty paths.

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
