package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doctorFixture builds a namespace and a set of probes that describe a
// healthy docktail app.
type doctorFixture struct {
	namespace *Namespace
}

const doctorTailnetStatus = `{
  "Self": {
    "DNSName": "pi.lemur-carat.ts.net.",
    "Tags": ["tag:server"]
  }
}`

func newDoctorFixture(t *testing.T, apps ...ApplicationSettings) doctorFixture {
	t.Helper()

	ns, err := NewNamespace("once", WithApplications(apps...))
	require.NoError(t, err)

	return doctorFixture{namespace: ns}
}

// healthyDocktailSettings describes a docktail app that is set up correctly.
func healthyDocktailSettings() ApplicationSettings {
	return ApplicationSettings{
		Name:      "writebook.069443",
		Image:     "ghcr.io/basecamp/writebook",
		Host:      "write.lemur-carat.ts.net",
		AppPort:   80,
		Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode, Service: "write", Port: 80, ServicePort: 443},
	}
}

// healthyDocktailContainer describes the container of a healthy docktail app.
func healthyDocktailContainer() DoctorContainer {
	return DoctorContainer{
		Name:  "once-app-writebook.069443-780dc3",
		State: "running",
		IP:    "172.18.0.4",
		Labels: map[string]string{
			docktailLabelEnable:      "true",
			docktailLabelName:        "write",
			docktailLabelPort:        "80",
			docktailLabelServicePort: "443",
		},
	}
}

// healthyServeStatus advertises the write service at the app's container.
func healthyServeStatus() []byte {
	return []byte(`{"Services":{"svc:write":{"Web":{"write.lemur-carat.ts.net:443":{"Handlers":{"/":{"Proxy":"http://172.18.0.4:80"}}}}}}}`)
}

// healthyProbes returns probes that pass every check.
func healthyProbes() DoctorProbes {
	return DoctorProbes{
		AppContainer: func(ctx context.Context, appName string) (DoctorContainer, error) {
			return healthyDocktailContainer(), nil
		},
		Containers: func(ctx context.Context) ([]DoctorContainer, error) {
			return []DoctorContainer{
				{Name: "docktail", State: "running"},
				healthyDocktailContainer(),
			}, nil
		},
		HTTPGet: func(ctx context.Context, url string) (int, error) {
			return 200, nil
		},
		TCPDial: func(ctx context.Context, address string) error {
			return nil
		},
		ServeStatus: func(ctx context.Context) ([]byte, error) {
			return healthyServeStatus(), nil
		},
		TailscaleStatus: func(ctx context.Context) ([]byte, error) {
			return []byte(doctorTailnetStatus), nil
		},
	}
}

// checkByLabel returns the check with the given label.
func checkByLabel(t *testing.T, report DoctorReport, label string) DoctorCheck {
	t.Helper()

	for _, check := range report.Checks {
		if check.Label == label {
			return check
		}
	}

	t.Fatalf("no check labeled %q in %v", label, report.Checks)
	return DoctorCheck{}
}

func TestDoctorHealthyDocktailApp(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	result, err := Doctor(context.Background(), fixture.namespace, "", healthyProbes())
	require.NoError(t, err)

	require.Len(t, result.Reports, 1)
	report := result.Reports[0]

	assert.Equal(t, "writebook.069443", report.AppName)
	assert.Equal(t, "write.lemur-carat.ts.net", report.Host)
	assert.False(t, report.Failed())
	assert.False(t, result.Failed())

	assert.Equal(t, DoctorPass, checkByLabel(t, report, "container").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "app port").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "docktail labels").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "backend port").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "docktail container").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "advertised").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "url").Status)

	assert.Contains(t, checkByLabel(t, report, "container").Message, "once-app-writebook.069443-780dc3")
	assert.Contains(t, checkByLabel(t, report, "advertised").Message, "http://172.18.0.4:80")
	assert.NotEmpty(t, report.Note)
}

func TestDoctorBackendPortNotListening(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.TCPDial = func(ctx context.Context, address string) error {
		return errors.New("connection refused")
	}
	probes.ListeningPorts = func(ctx context.Context, containerName string) ([]int, error) {
		return []int{80}, nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	require.Len(t, result.Reports, 1)
	report := result.Reports[0]

	backend := checkByLabel(t, report, "backend port")
	assert.Equal(t, DoctorFail, backend.Status)
	assert.Contains(t, backend.Message, "nothing is listening inside the container")
	assert.Contains(t, backend.Message, "80")
	assert.Equal(t, "set the Tailscale backend port to the port the app listens on (80)", backend.Remedy)

	assert.True(t, report.Failed())
	assert.True(t, result.Failed())

	// Every other check still ran and passed.
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "container").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "app port").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "advertised").Status)
}

func TestDoctorBackendPortNotListeningReportsAppPort(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.TCPDial = func(ctx context.Context, address string) error {
		return errors.New("connection refused")
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	backend := checkByLabel(t, result.Reports[0], "backend port")
	assert.Equal(t, "set the Tailscale backend port to the port the app listens on (80)", backend.Remedy)
	assert.NotContains(t, backend.Message, "listening:")
}

func TestDoctorServiceNotAdvertised(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.ServeStatus = func(ctx context.Context) ([]byte, error) {
		return []byte(`{"Services":{}}`), nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	advertised := checkByLabel(t, result.Reports[0], "advertised")
	assert.Equal(t, DoctorFail, advertised.Status)
	assert.Contains(t, advertised.Message, "svc:write is not advertised")
	assert.NotEmpty(t, advertised.Remedy)
}

func TestDoctorAdvertisementPointsElsewhere(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.ServeStatus = func(ctx context.Context) ([]byte, error) {
		return []byte(`{"Services":{"svc:write":{"Web":{"write.lemur-carat.ts.net:443":{"Handlers":{"/":{"Proxy":"http://172.18.0.9:80"}}}}}}}`), nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	advertised := checkByLabel(t, result.Reports[0], "advertised")
	assert.Equal(t, DoctorFail, advertised.Status)
	assert.Contains(t, advertised.Message, "172.18.0.9")
	assert.Contains(t, advertised.Message, "172.18.0.4")
}

func TestDoctorAdvertisementPointsAtAnotherPort(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.ServeStatus = func(ctx context.Context) ([]byte, error) {
		return []byte(`{"Services":{"svc:write":{"Web":{"write.lemur-carat.ts.net:443":{"Handlers":{"/":{"Proxy":"http://172.18.0.4:7878"}}}}}}}`), nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	advertised := checkByLabel(t, result.Reports[0], "advertised")
	assert.Equal(t, DoctorFail, advertised.Status)
	assert.Contains(t, advertised.Message, "172.18.0.4:7878")
}

func TestDoctorLabelsDisagreeWithSettings(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		container := healthyDocktailContainer()
		container.Labels[docktailLabelPort] = "7878"
		container.Labels[docktailLabelServicePort] = "8443"
		return container, nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	labels := checkByLabel(t, result.Reports[0], "docktail labels")
	assert.Equal(t, DoctorFail, labels.Status)
	assert.Contains(t, labels.Message, "7878")
	assert.Contains(t, labels.Message, "8443")
	assert.NotEmpty(t, labels.Remedy)
}

func TestDoctorLabelsMissing(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		container := healthyDocktailContainer()
		delete(container.Labels, docktailLabelName)
		return container, nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	labels := checkByLabel(t, result.Reports[0], "docktail labels")
	assert.Equal(t, DoctorFail, labels.Status)
	assert.Contains(t, labels.Message, "docktail.service.name is missing")
}

func TestDoctorNonDocktailApp(t *testing.T) {
	fixture := newDoctorFixture(t, ApplicationSettings{
		Name:    "blog",
		Image:   "ghcr.io/basecamp/blog",
		Host:    "blog.example.com",
		AppPort: 3000,
	})

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		return DoctorContainer{Name: "once-app-blog-abc123", State: "running", IP: "172.18.0.7"}, nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	require.Len(t, result.Reports, 1)
	report := result.Reports[0]

	assert.False(t, report.Failed())
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "container").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "app port").Status)
	assert.Equal(t, "https://blog.example.com", report.URL)
	assert.Empty(t, report.Note)

	// No docktail checks are reported for an app that doesn't use docktail.
	for _, check := range report.Checks {
		assert.NotContains(t, []string{"docktail labels", "backend port", "docktail container", "advertised"}, check.Label)
	}
}

func TestDoctorAppNotRunning(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		return DoctorContainer{Name: "once-app-writebook.069443-780dc3", State: "exited"}, nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	report := result.Reports[0]

	container := checkByLabel(t, report, "container")
	assert.Equal(t, DoctorFail, container.Status)
	assert.Contains(t, container.Message, "exited")
	assert.NotEmpty(t, container.Remedy)

	assert.Equal(t, DoctorSkip, checkByLabel(t, report, "app port").Status)
	assert.Equal(t, DoctorSkip, checkByLabel(t, report, "backend port").Status)

	assert.True(t, report.Failed())
}

func TestDoctorAppWithoutContainer(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		return DoctorContainer{}, fmt.Errorf("no container found for app %s", appName)
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	container := checkByLabel(t, result.Reports[0], "container")
	assert.Equal(t, DoctorFail, container.Status)
	assert.Contains(t, container.Message, "not found")
	assert.NotEmpty(t, container.Remedy)
}

func TestDoctorDocktailContainerNotRunning(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.Containers = func(ctx context.Context) ([]DoctorContainer, error) {
		return []DoctorContainer{{Name: "docktail", State: "exited"}}, nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	docktail := checkByLabel(t, result.Reports[0], "docktail container")
	assert.Equal(t, DoctorFail, docktail.Status)
	assert.Contains(t, docktail.Message, "no running docktail container")
	assert.NotEmpty(t, docktail.Remedy)
}

func TestDoctorMultipleAppsWithOneFailing(t *testing.T) {
	fixture := newDoctorFixture(t,
		healthyDocktailSettings(),
		ApplicationSettings{
			Name:    "blog",
			Image:   "ghcr.io/basecamp/blog",
			Host:    "blog.example.com",
			AppPort: 3000,
		},
	)

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		if appName == "blog" {
			return DoctorContainer{Name: "once-app-blog-abc123", State: "exited"}, nil
		}
		return healthyDocktailContainer(), nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	require.Len(t, result.Reports, 2)
	assert.True(t, result.Failed())

	// The blog app's container is stopped, so it fails; the docktail app
	// is healthy.
	assert.True(t, result.Reports[0].Failed())
	assert.False(t, result.Reports[1].Failed())
}

func TestDoctorSelectsOneAppByHost(t *testing.T) {
	fixture := newDoctorFixture(t,
		healthyDocktailSettings(),
		ApplicationSettings{
			Name:  "blog",
			Image: "ghcr.io/basecamp/blog",
			Host:  "blog.example.com",
		},
	)

	result, err := Doctor(context.Background(), fixture.namespace, "blog.example.com", healthyProbes())
	require.NoError(t, err)

	require.Len(t, result.Reports, 1)
	assert.Equal(t, "blog", result.Reports[0].AppName)
}

func TestDoctorSelectsOneAppByName(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	result, err := Doctor(context.Background(), fixture.namespace, "writebook.069443", healthyProbes())
	require.NoError(t, err)

	require.Len(t, result.Reports, 1)
	assert.Equal(t, "writebook.069443", result.Reports[0].AppName)
}

func TestDoctorUnknownHost(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	_, err := Doctor(context.Background(), fixture.namespace, "nope.example.com", healthyProbes())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDoctorUnknownHost)
	assert.Contains(t, err.Error(), "nope.example.com")
}

func TestDoctorAppPortAnswersNonSuccess(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.HTTPGet = func(ctx context.Context, url string) (int, error) {
		return 503, nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	appPort := checkByLabel(t, result.Reports[0], "app port")
	assert.Equal(t, DoctorFail, appPort.Status)
	assert.Contains(t, appPort.Message, "503")
	assert.NotEmpty(t, appPort.Remedy)
}

func TestDoctorURLUsesMagicDNSSuffix(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	result, err := Doctor(context.Background(), fixture.namespace, "", healthyProbes())
	require.NoError(t, err)

	url := checkByLabel(t, result.Reports[0], "url")
	assert.Equal(t, "https://write.lemur-carat.ts.net", url.Message)
	assert.NotContains(t, url.Message, "http://")
}

func TestDoctorURLIsAlwaysHTTPS(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.TailscaleStatus = func(ctx context.Context) ([]byte, error) {
		return []byte(`{"Self":{"DNSName":"pi.example.ts.net."}}`), nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	url := checkByLabel(t, result.Reports[0], "url")
	assert.Equal(t, "https://write.example.ts.net", url.Message)
	assert.NotContains(t, url.Message, "http://")
}

func TestDoctorURLFallsBackWithoutMagicDNS(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.TailscaleStatus = func(ctx context.Context) ([]byte, error) {
		return nil, errors.New("tailscale not installed")
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	url := checkByLabel(t, result.Reports[0], "url")
	assert.Equal(t, DoctorSkip, url.Status)
}

func TestDoctorProbesForNamespace(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := DoctorProbesFor(fixture.namespace)

	require.NotNil(t, probes.AppContainer)
	require.NotNil(t, probes.Containers)
	require.NotNil(t, probes.HTTPGet)
	require.NotNil(t, probes.TCPDial)
	require.NotNil(t, probes.ServeStatus)
	require.NotNil(t, probes.TailscaleStatus)
	require.NotNil(t, probes.ListeningPorts)
}

func TestDoctorRemedyMentionsBackendPortSetting(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.TCPDial = func(ctx context.Context, address string) error {
		return errors.New("connection refused")
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	backend := checkByLabel(t, result.Reports[0], "backend port")
	assert.True(t, strings.Contains(backend.Remedy, "backend port"))
}

func TestDoctorDirectModeDialsTheContainerIP(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	var dialled string
	probes := healthyProbes()
	probes.TCPDial = func(ctx context.Context, address string) error {
		dialled = address
		return nil
	}

	_, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	assert.Equal(t, "172.18.0.4:80", dialled)
}

func TestDoctorIndirectModeDialsTheHostLoopback(t *testing.T) {
	settings := healthyDocktailSettings()
	direct := false
	settings.Tailscale.Direct = &direct

	fixture := newDoctorFixture(t, settings)

	var dialled string
	probes := healthyProbes()
	probes.TCPDial = func(ctx context.Context, address string) error {
		dialled = address
		return nil
	}

	_, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:80", dialled)
}

func TestDoctorIndirectModeAcceptsANonContainerHost(t *testing.T) {
	settings := healthyDocktailSettings()
	direct := false
	settings.Tailscale.Direct = &direct

	fixture := newDoctorFixture(t, settings)

	probes := healthyProbes()
	probes.ServeStatus = func(ctx context.Context) ([]byte, error) {
		return []byte(`{"Services":{"svc:write":{"Web":{"write.lemur-carat.ts.net:443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:80"}}}}}}}`), nil
	}

	result, err := Doctor(context.Background(), fixture.namespace, "", probes)
	require.NoError(t, err)

	assert.Equal(t, DoctorPass, checkByLabel(t, result.Reports[0], "advertised").Status)
}
