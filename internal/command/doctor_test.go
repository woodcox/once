package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func doctorReportFixture(appName, host string, checks ...docker.DoctorCheck) docker.DoctorReport {
	return docker.DoctorReport{
		AppName: appName,
		Host:    host,
		Checks:  checks,
	}
}

func TestRenderDoctorReportHealthyApp(t *testing.T) {
	report := doctorReportFixture("writebook.069443", "write.lemur-carat.ts.net",
		docker.DoctorCheck{Label: "container", Status: docker.DoctorPass, Message: "container running (once-app-writebook.069443-780dc3)"},
		docker.DoctorCheck{Label: "app port", Status: docker.DoctorPass, Message: "port 80 answers /up with 200"},
		docker.DoctorCheck{Label: "url", Status: docker.DoctorPass, Message: "https://write.lemur-carat.ts.net"},
	)
	report.URL = "https://write.lemur-carat.ts.net"

	var out bytes.Buffer
	renderDoctorReport(&out, docker.DoctorResult{Reports: []docker.DoctorReport{report}})

	output := out.String()

	assert.Contains(t, output, "writebook.069443 (write.lemur-carat.ts.net)")
	assert.Contains(t, output, "ok")
	assert.Contains(t, output, "container running (once-app-writebook.069443-780dc3)")
	assert.Contains(t, output, "url")
	assert.Contains(t, output, "https://write.lemur-carat.ts.net")
}

func TestRenderDoctorReportFailedCheckShowsRemedy(t *testing.T) {
	report := doctorReportFixture("writebook.069443", "write.lemur-carat.ts.net",
		docker.DoctorCheck{Label: "backend port", Status: docker.DoctorFail, Message: "port 7878: nothing is listening inside the container", Remedy: "set the Tailscale backend port to the port the app listens on (80)"},
	)

	var out bytes.Buffer
	renderDoctorReport(&out, docker.DoctorResult{Reports: []docker.DoctorReport{report}})

	output := out.String()

	assert.Contains(t, output, "FAIL")
	assert.Contains(t, output, "nothing is listening inside the container")
	assert.Contains(t, output, "remedy: set the Tailscale backend port to the port the app listens on (80)")
}

func TestRenderDoctorReportSkippedCheckHasNoRemedy(t *testing.T) {
	report := doctorReportFixture("writebook.069443", "write.lemur-carat.ts.net",
		docker.DoctorCheck{Label: "app port", Status: docker.DoctorSkip, Message: "container is not running"},
	)

	var out bytes.Buffer
	renderDoctorReport(&out, docker.DoctorResult{Reports: []docker.DoctorReport{report}})

	assert.Contains(t, out.String(), "n/a")
	assert.NotContains(t, out.String(), "remedy:")
}

func TestRenderDoctorReportShowsNote(t *testing.T) {
	report := doctorReportFixture("writebook.069443", "write.lemur-carat.ts.net",
		docker.DoctorCheck{Label: "container", Status: docker.DoctorPass, Message: "container running"},
	)
	report.Note = "service approval and client grants live in the Tailscale admin console"

	var out bytes.Buffer
	renderDoctorReport(&out, docker.DoctorResult{Reports: []docker.DoctorReport{report}})

	assert.Contains(t, out.String(), "note: service approval and client grants live in the Tailscale admin console")
}

func TestRenderDoctorReportNeverShowsInsecureURL(t *testing.T) {
	writebook := doctorReportFixture("writebook.069443", "write.lemur-carat.ts.net",
		docker.DoctorCheck{Label: "advertised", Status: docker.DoctorPass, Message: ": svc:write -> http://172.18.0.4:80"},
		docker.DoctorCheck{Label: "url", Status: docker.DoctorPass, Message: "https://write.lemur-carat.ts.net"},
	)
	writebook.URL = "https://write.lemur-carat.ts.net"

	blog := doctorReportFixture("blog", "blog.example.com",
		docker.DoctorCheck{Label: "url", Status: docker.DoctorPass, Message: "https://blog.example.com"},
	)
	blog.URL = "https://blog.example.com"

	reports := []docker.DoctorReport{writebook, blog}

	var out bytes.Buffer
	renderDoctorReport(&out, docker.DoctorResult{Reports: reports})

	output := out.String()

	assert.Contains(t, output, "https://write.lemur-carat.ts.net")
	assert.Contains(t, output, "https://blog.example.com")

	// The only URL line is the app's public URL, and it is always https.
	urlLines := []string{}
	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, " url ") && strings.Contains(line, "://") {
			urlLines = append(urlLines, line)
		}
	}

	require.Len(t, urlLines, 2)
	for _, line := range urlLines {
		assert.Contains(t, line, "https://")
		assert.NotContains(t, line, "http://")
	}
}

func TestRenderDoctorReportURLIsAlwaysHTTPS(t *testing.T) {
	// Whatever the report says, the URL line the doctor prints is the
	// app's https URL.
	report := doctorReportFixture("blog", "blog.example.com",
		docker.DoctorCheck{Label: "container", Status: docker.DoctorPass, Message: "running (once-app-blog-abc123)"},
	)
	report.URL = "https://blog.example.com"

	var out bytes.Buffer
	renderDoctorReport(&out, docker.DoctorResult{Reports: []docker.DoctorReport{report}})

	output := out.String()

	assert.Contains(t, output, "url")
	assert.Contains(t, output, "https://blog.example.com")
	assert.NotContains(t, output, "http://")
}

func TestRenderDoctorReportMultipleApps(t *testing.T) {
	result := docker.DoctorResult{Reports: []docker.DoctorReport{
		doctorReportFixture("blog", "blog.example.com",
			docker.DoctorCheck{Label: "container", Status: docker.DoctorPass, Message: "container running"},
		),
		doctorReportFixture("writebook.069443", "write.lemur-carat.ts.net",
			docker.DoctorCheck{Label: "container", Status: docker.DoctorFail, Message: "container is exited", Remedy: "start the app"},
		),
	}}

	var out bytes.Buffer
	renderDoctorReport(&out, result)

	output := out.String()

	assert.Contains(t, output, "blog (blog.example.com)")
	assert.Contains(t, output, "writebook.069443 (write.lemur-carat.ts.net)")
	assert.True(t, result.Failed())
}

func TestDoctorExitErrorExitCode(t *testing.T) {
	code, ok := ExitCode(doctorExitError{})
	require.True(t, ok)
	assert.Equal(t, 1, code)
}
