package docker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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

// apps returns the namespace's applications, as a caller would snapshot them.
func (f doctorFixture) apps() []*Application {
	return f.namespace.Applications()
}

// healthyDocktailSettings describes a docktail app that is set up correctly.
func healthyDocktailSettings() ApplicationSettings {
	return ApplicationSettings{
		Name:      "writebook.069443",
		Image:     "ghcr.io/basecamp/writebook",
		Host:      "write.lemur-carat.ts.net",
		AppPort:   80,
		Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode, Service: "write", ServicePort: 443},
	}
}

// healthyDocktailContainer describes the container of a healthy docktail app.
func healthyDocktailContainer() DoctorContainer {
	settings := healthyDocktailSettings()
	return DoctorContainer{
		Name:   "once-app-writebook.069443-780dc3",
		State:  "running",
		IP:     "172.18.0.4",
		Labels: settings.DocktailLabels(),
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

	result, err := Doctor(context.Background(), fixture.apps(), "", healthyProbes())
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
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "docktail container").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "advertised").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "url").Status)

	assert.Contains(t, checkByLabel(t, report, "container").Message, "once-app-writebook.069443-780dc3")
	assert.Contains(t, checkByLabel(t, report, "advertised").Message, "http://172.18.0.4:80")
	assert.NotEmpty(t, report.Note)
}

func TestDoctorAppPortFailureListsWhatIsListening(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.HTTPGet = func(ctx context.Context, url string) (int, error) {
		return 0, errors.New("connection refused")
	}
	probes.ListeningPorts = func(ctx context.Context, containerName string) ([]int, error) {
		return []int{8080}, nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	require.Len(t, result.Reports, 1)
	report := result.Reports[0]

	appPort := checkByLabel(t, report, "app port")
	assert.Equal(t, DoctorFail, appPort.Status)
	assert.Contains(t, appPort.Message, "80 did not answer")
	assert.Contains(t, appPort.Message, "(listening: 8080)")
	assert.Equal(t, "make sure the app listens on port 80 inside the container", appPort.Remedy)

	assert.True(t, report.Failed())
	assert.True(t, result.Failed())

	// Checks that don't depend on the app answering still ran and passed.
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "container").Status)
	assert.Equal(t, DoctorPass, checkByLabel(t, report, "advertised").Status)
}

func TestDoctorAppPortFailureWithoutListeningPorts(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.HTTPGet = func(ctx context.Context, url string) (int, error) {
		return 0, errors.New("connection refused")
	}
	probes.ListeningPorts = func(ctx context.Context, containerName string) ([]int, error) {
		return nil, nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	appPort := checkByLabel(t, result.Reports[0], "app port")
	assert.Equal(t, DoctorFail, appPort.Status)
	assert.NotContains(t, appPort.Message, "listening:")
}

func TestDoctorServiceNotAdvertised(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.ServeStatus = func(ctx context.Context) ([]byte, error) {
		return []byte(`{"Services":{}}`), nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
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

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
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

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
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

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
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

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	labels := checkByLabel(t, result.Reports[0], "docktail labels")
	assert.Equal(t, DoctorFail, labels.Status)
	assert.Contains(t, labels.Message, "docktail.service.name is missing")
}

func TestDoctorLabelsCatchProtocolMismatch(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		container := healthyDocktailContainer()
		container.Labels[docktailLabelProtocol] = "https"
		return container, nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	labels := checkByLabel(t, result.Reports[0], "docktail labels")
	assert.Equal(t, DoctorFail, labels.Status)
	assert.Contains(t, labels.Message, "docktail.service.protocol")
}

func TestDoctorLabelsCatchDirectMismatch(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		container := healthyDocktailContainer()
		container.Labels[docktailLabelDirect] = "false"
		return container, nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	labels := checkByLabel(t, result.Reports[0], "docktail labels")
	assert.Equal(t, DoctorFail, labels.Status)
	assert.Contains(t, labels.Message, "docktail.service.direct")
}

func TestDoctorLabelsCatchTagsMismatch(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		container := healthyDocktailContainer()
		container.Labels[docktailLabelTags] = "tag:server"
		return container, nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	labels := checkByLabel(t, result.Reports[0], "docktail labels")
	assert.Equal(t, DoctorFail, labels.Status)
	assert.Contains(t, labels.Message, "docktail.tags")
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

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
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
		assert.NotContains(t, []string{"docktail labels", "docktail container", "advertised"}, check.Label)
	}
}

func TestDoctorAppNotRunning(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		return DoctorContainer{Name: "once-app-writebook.069443-780dc3", State: "exited"}, nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	report := result.Reports[0]

	container := checkByLabel(t, report, "container")
	assert.Equal(t, DoctorFail, container.Status)
	assert.Contains(t, container.Message, "exited")
	assert.NotEmpty(t, container.Remedy)

	assert.Equal(t, DoctorSkip, checkByLabel(t, report, "app port").Status)

	assert.True(t, report.Failed())
}

func TestDoctorAppWithoutContainer(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		return DoctorContainer{}, fmt.Errorf("%w %s", ErrDoctorAppNotFound, appName)
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	container := checkByLabel(t, result.Reports[0], "container")
	assert.Equal(t, DoctorFail, container.Status)
	assert.Contains(t, container.Message, "not found")
	assert.NotEmpty(t, container.Remedy)
	assert.Contains(t, container.Remedy, "redeploy")
}

func TestDoctorAppContainerProbeFailure(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		return DoctorContainer{}, errors.New("docker daemon is unreachable")
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	container := checkByLabel(t, result.Reports[0], "container")
	assert.Equal(t, DoctorFail, container.Status)
	assert.Contains(t, container.Message, "docker daemon is unreachable")
	assert.Equal(t, "make sure the Docker daemon is reachable", container.Remedy)

	// The checks that need the container are skipped rather than reporting
	// the same failure again.
	assert.Equal(t, DoctorSkip, checkByLabel(t, result.Reports[0], "app port").Status)
}

func TestDoctorContainerProbeUnavailable(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.AppContainer = nil

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	container := checkByLabel(t, result.Reports[0], "container")
	assert.Equal(t, DoctorSkip, container.Status)
}

func TestDoctorDocktailContainerNotRunning(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.Containers = func(ctx context.Context) ([]DoctorContainer, error) {
		return []DoctorContainer{{Name: "docktail", State: "exited"}}, nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	docktail := checkByLabel(t, result.Reports[0], "docktail container")
	assert.Equal(t, DoctorFail, docktail.Status)
	assert.Contains(t, docktail.Message, "no running docktail container")
	assert.NotEmpty(t, docktail.Remedy)
}

func TestDoctorDocktailContainerFoundByImage(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.Containers = func(ctx context.Context) ([]DoctorContainer, error) {
		return []DoctorContainer{{Name: "project-docktail-1", State: "running", Image: "ghcr.io/marvinvr/docktail:latest"}}, nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	assert.Equal(t, DoctorPass, checkByLabel(t, result.Reports[0], "docktail container").Status)
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

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
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

	result, err := Doctor(context.Background(), fixture.apps(), "blog.example.com", healthyProbes())
	require.NoError(t, err)

	require.Len(t, result.Reports, 1)
	assert.Equal(t, "blog", result.Reports[0].AppName)
}

func TestDoctorSelectsOneAppByName(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	result, err := Doctor(context.Background(), fixture.apps(), "writebook.069443", healthyProbes())
	require.NoError(t, err)

	require.Len(t, result.Reports, 1)
	assert.Equal(t, "writebook.069443", result.Reports[0].AppName)
}

func TestDoctorUnknownHost(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	_, err := Doctor(context.Background(), fixture.apps(), "nope.example.com", healthyProbes())

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

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	appPort := checkByLabel(t, result.Reports[0], "app port")
	assert.Equal(t, DoctorFail, appPort.Status)
	assert.Contains(t, appPort.Message, "503")
	assert.NotEmpty(t, appPort.Remedy)
}

func TestDoctorAppPortHTTPSBackend(t *testing.T) {
	settings := healthyDocktailSettings()
	settings.Tailscale.Protocol = "https"
	settings.AppPort = 443

	fixture := newDoctorFixture(t, settings)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "https://"))
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		container := healthyDocktailContainer()
		container.IP = host
		container.State = "running"
		return container, nil
	}

	// Override the settings in the namespace so the app reports the server's port.
	app := fixture.namespace.Applications()[0]
	app.Settings.AppPort = port
	app.Settings.Tailscale.Protocol = "https"

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	appPort := checkByLabel(t, result.Reports[0], "app port")
	assert.Equal(t, DoctorPass, appPort.Status)
}

func TestDoctorAppPortTCPBackend(t *testing.T) {
	settings := healthyDocktailSettings()
	settings.Tailscale.Protocol = "tcp"
	settings.AppPort = 5432

	fixture := newDoctorFixture(t, settings)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	probes := healthyProbes()
	probes.AppContainer = func(ctx context.Context, appName string) (DoctorContainer, error) {
		container := healthyDocktailContainer()
		container.IP = "127.0.0.1"
		container.State = "running"
		return container, nil
	}
	probes.TCPDial = func(ctx context.Context, address string) error {
		_, err := net.DialTimeout("tcp", address, doctorDialTimeout)
		return err
	}

	app := fixture.namespace.Applications()[0]
	app.Settings.AppPort = port
	app.Settings.Tailscale.Protocol = "tcp"

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	appPort := checkByLabel(t, result.Reports[0], "app port")
	assert.Equal(t, DoctorPass, appPort.Status)
}

func TestDoctorAppPortNothingListens(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.HTTPGet = func(ctx context.Context, url string) (int, error) {
		return 0, errors.New("connection refused")
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	appPort := checkByLabel(t, result.Reports[0], "app port")
	assert.Equal(t, DoctorFail, appPort.Status)
	assert.Contains(t, appPort.Message, "did not answer")
}

func TestDoctorURLUsesMagicDNSSuffix(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	result, err := Doctor(context.Background(), fixture.apps(), "", healthyProbes())
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

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
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

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	url := checkByLabel(t, result.Reports[0], "url")
	assert.Equal(t, DoctorSkip, url.Status)
}

func TestDoctorURLReportsTCPEndpointWithoutMagicDNS(t *testing.T) {
	settings := healthyDocktailSettings()
	settings.Tailscale.ServiceProtocol = "tcp"
	settings.Tailscale.ServicePort = 5432
	settings.AppPort = 5432
	fixture := newDoctorFixture(t, settings)

	probes := healthyProbes()
	probes.TailscaleStatus = func(ctx context.Context) ([]byte, error) {
		return nil, errors.New("tailscale not installed")
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	report := result.Reports[0]
	url := checkByLabel(t, report, "url")
	assert.Equal(t, DoctorPass, url.Status)
	assert.Equal(t, "tcp:write:5432", url.Message)
	assert.Equal(t, "tcp:write:5432", report.URL)
}

func TestDoctorURLReportsTLSTerminatedTCPEndpoint(t *testing.T) {
	settings := healthyDocktailSettings()
	settings.Tailscale.ServiceProtocol = "tls-terminated-tcp"
	settings.Tailscale.ServicePort = 5432
	settings.AppPort = 5432
	fixture := newDoctorFixture(t, settings)

	probes := healthyProbes()
	probes.TailscaleStatus = func(ctx context.Context) ([]byte, error) {
		return nil, errors.New("tailscale not installed")
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	report := result.Reports[0]
	url := checkByLabel(t, report, "url")
	assert.Equal(t, DoctorPass, url.Status)
	assert.Equal(t, "tcp:write:5432", url.Message)
	assert.Equal(t, "tcp:write:5432", report.URL)
}

func TestDoctorURLIncludesNonDefaultPorts(t *testing.T) {
	t.Run("https on 8443", func(t *testing.T) {
		settings := healthyDocktailSettings()
		settings.Tailscale.ServiceProtocol = "https"
		settings.Tailscale.ServicePort = 8443
		fixture := newDoctorFixture(t, settings)

		result, err := Doctor(context.Background(), fixture.apps(), "", healthyProbes())
		require.NoError(t, err)

		url := checkByLabel(t, result.Reports[0], "url")
		assert.Equal(t, "https://write.lemur-carat.ts.net:8443", url.Message)
	})

	t.Run("http on 8080", func(t *testing.T) {
		settings := healthyDocktailSettings()
		settings.Tailscale.ServiceProtocol = "http"
		settings.Tailscale.ServicePort = 8080
		fixture := newDoctorFixture(t, settings)

		result, err := Doctor(context.Background(), fixture.apps(), "", healthyProbes())
		require.NoError(t, err)

		url := checkByLabel(t, result.Reports[0], "url")
		assert.Equal(t, "http://write.lemur-carat.ts.net:8080", url.Message)
	})
}

func TestDoctorURLReportsTCPServicesAsEndpoints(t *testing.T) {
	settings := healthyDocktailSettings()
	settings.Tailscale.ServiceProtocol = "tcp"
	settings.Tailscale.ServicePort = 5432
	settings.AppPort = 5432
	fixture := newDoctorFixture(t, settings)

	result, err := Doctor(context.Background(), fixture.apps(), "", healthyProbes())
	require.NoError(t, err)

	url := checkByLabel(t, result.Reports[0], "url")
	assert.Equal(t, "tcp:write:5432", url.Message)
	assert.Equal(t, "tcp:write:5432", result.Reports[0].URL)
}

func TestDoctorBoundsEveryProbe(t *testing.T) {
	shrinkDoctorTimeouts(t, 50*time.Millisecond)

	fixture := newDoctorFixture(t, healthyDocktailSettings())

	// Every probe that can hang blocks until its context is cancelled; one
	// the doctor forgot to bound would hang the whole check.
	block := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	probes := healthyProbes()
	probes.Containers = func(ctx context.Context) ([]DoctorContainer, error) {
		return nil, block(ctx)
	}
	probes.ServeStatus = func(ctx context.Context) ([]byte, error) {
		return nil, block(ctx)
	}
	probes.TailscaleStatus = func(ctx context.Context) ([]byte, error) {
		return nil, block(ctx)
	}
	probes.ListeningPorts = func(ctx context.Context, containerName string) ([]int, error) {
		return nil, block(ctx)
	}
	probes.TCPDial = func(ctx context.Context, address string) error {
		return errors.New("connection refused")
	}
	probes.HTTPGet = func(ctx context.Context, url string) (int, error) {
		return 0, block(ctx)
	}

	start := time.Now()
	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	assert.Less(t, time.Since(start), 5*time.Second, "every blocked probe was bounded")
	require.Len(t, result.Reports, 1)

	// The hung probes are reported as failed checks rather than a hang.
	assert.Equal(t, DoctorFail, checkByLabel(t, result.Reports[0], "docktail container").Status)
	assert.Equal(t, DoctorFail, checkByLabel(t, result.Reports[0], "advertised").Status)
	assert.Equal(t, DoctorFail, checkByLabel(t, result.Reports[0], "app port").Status)
	assert.Equal(t, DoctorSkip, checkByLabel(t, result.Reports[0], "url").Status)
}

func TestDoctorRunsAgainstTheSnapshotItIsGiven(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())
	apps := fixture.apps()

	// A namespace refresh replaces the application slice wholesale; the
	// check still runs against the snapshot taken beforehand.
	fixture.namespace.applications = nil

	result, err := Doctor(context.Background(), apps, "", healthyProbes())
	require.NoError(t, err)

	require.Len(t, result.Reports, 1)
	assert.Equal(t, "writebook.069443", result.Reports[0].AppName)
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

func TestDoctorRemedyNamesTheAppPort(t *testing.T) {
	fixture := newDoctorFixture(t, healthyDocktailSettings())

	probes := healthyProbes()
	probes.HTTPGet = func(ctx context.Context, url string) (int, error) {
		return 0, errors.New("connection refused")
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	appPort := checkByLabel(t, result.Reports[0], "app port")
	assert.Contains(t, appPort.Remedy, "port 80")
}

func TestDoctorProbesTheContainerIP(t *testing.T) {
	// The app port probe goes to the container's own address on the Docker
	// network — never loopback, since docktail proxies to the container.
	t.Run("http", func(t *testing.T) {
		fixture := newDoctorFixture(t, healthyDocktailSettings())

		var fetched string
		probes := healthyProbes()
		probes.HTTPGet = func(ctx context.Context, url string) (int, error) {
			fetched = url
			return 200, nil
		}

		_, err := Doctor(context.Background(), fixture.apps(), "", probes)
		require.NoError(t, err)

		assert.Contains(t, fetched, "172.18.0.4:80")
	})

	t.Run("tcp", func(t *testing.T) {
		settings := healthyDocktailSettings()
		settings.Tailscale.Protocol = "tcp"
		settings.Tailscale.ServiceProtocol = "tcp"
		settings.Tailscale.ServicePort = 5432
		settings.AppPort = 5432
		fixture := newDoctorFixture(t, settings)

		var dialled string
		probes := healthyProbes()
		probes.TCPDial = func(ctx context.Context, address string) error {
			dialled = address
			return nil
		}

		_, err := Doctor(context.Background(), fixture.apps(), "", probes)
		require.NoError(t, err)

		assert.Equal(t, "172.18.0.4:5432", dialled)
	})
}

func TestDoctorTCPServicesAreAdvertised(t *testing.T) {
	settings := healthyDocktailSettings()
	settings.Tailscale.ServiceProtocol = "tcp"
	settings.Tailscale.ServicePort = 5432
	settings.AppPort = 5432

	fixture := newDoctorFixture(t, settings)

	probes := healthyProbes()
	probes.ServeStatus = func(ctx context.Context) ([]byte, error) {
		return []byte(`{"Services":{"svc:write":{"TCP":{"5432":{"TCPForward":"172.18.0.4:5432"}}}}}`), nil
	}
	probes.TCPDial = func(ctx context.Context, address string) error {
		return nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	advertised := checkByLabel(t, result.Reports[0], "advertised")
	assert.Equal(t, DoctorPass, advertised.Status)
	assert.Contains(t, advertised.Message, "tcp://172.18.0.4:5432")
}

func TestDoctorTCPServicesFailWithWrongForwardPort(t *testing.T) {
	settings := healthyDocktailSettings()
	settings.Tailscale.ServiceProtocol = "tcp"
	settings.Tailscale.ServicePort = 5432
	settings.AppPort = 5432

	fixture := newDoctorFixture(t, settings)

	probes := healthyProbes()
	probes.ServeStatus = func(ctx context.Context) ([]byte, error) {
		return []byte(`{"Services":{"svc:write":{"TCP":{"5432":{"TCPForward":"172.18.0.4:5433"}}}}}`), nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	advertised := checkByLabel(t, result.Reports[0], "advertised")
	assert.Equal(t, DoctorFail, advertised.Status)
	assert.Contains(t, advertised.Message, "5433")
}

func TestDoctorTCPServicesFailWithWrongForwardHost(t *testing.T) {
	settings := healthyDocktailSettings()
	settings.Tailscale.ServiceProtocol = "tcp"
	settings.Tailscale.ServicePort = 5432
	settings.AppPort = 5432

	fixture := newDoctorFixture(t, settings)

	probes := healthyProbes()
	probes.ServeStatus = func(ctx context.Context) ([]byte, error) {
		return []byte(`{"Services":{"svc:write":{"TCP":{"5432":{"TCPForward":"172.18.0.9:5432"}}}}}`), nil
	}

	result, err := Doctor(context.Background(), fixture.apps(), "", probes)
	require.NoError(t, err)

	advertised := checkByLabel(t, result.Reports[0], "advertised")
	assert.Equal(t, DoctorFail, advertised.Status)
	assert.Contains(t, advertised.Message, "172.18.0.9")
}

// shrinkDoctorTimeouts shrinks the doctor's probe budgets for the duration
// of the test, so a bounded probe gives up in milliseconds.
func shrinkDoctorTimeouts(t *testing.T, d time.Duration) {
	t.Helper()

	origProbe, origExec := doctorProbeTimeout, doctorExecTimeout
	doctorProbeTimeout, doctorExecTimeout = d, d
	t.Cleanup(func() {
		doctorProbeTimeout, doctorExecTimeout = origProbe, origExec
	})
}
