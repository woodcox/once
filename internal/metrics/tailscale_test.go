package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTailscaleScraperParseMetrics(t *testing.T) {
	input := `# HELP tailscaled_serve_inbound_bytes_total Bytes received from peers on Serve connections for Tailscale Services
# TYPE tailscaled_serve_inbound_bytes_total counter
tailscaled_serve_inbound_bytes_total{service="svc:write"} 2928127
tailscaled_serve_inbound_bytes_total{service="svc:budget"} 39443
# HELP tailscaled_serve_outbound_bytes_total Bytes sent to peers on Serve connections for Tailscale Services
# TYPE tailscaled_serve_outbound_bytes_total counter
tailscaled_serve_outbound_bytes_total{service="svc:write"} 24582988
tailscaled_serve_outbound_bytes_total{service="svc:budget"} 156904
`

	scraper := NewTailscaleScraper(TailscaleScraperSettings{})
	counters, err := scraper.parseMetrics(strings.NewReader(input))

	require.NoError(t, err)
	require.Len(t, counters, 2)

	assert.Equal(t, float64(2928127), counters["write"].inbound)
	assert.Equal(t, float64(24582988), counters["write"].outbound)
	assert.Equal(t, float64(39443), counters["budget"].inbound)
	assert.Equal(t, float64(156904), counters["budget"].outbound)
}

func TestTailscaleScraperParseMetricsEmpty(t *testing.T) {
	scraper := NewTailscaleScraper(TailscaleScraperSettings{})
	counters, err := scraper.parseMetrics(strings.NewReader(""))

	require.NoError(t, err)
	assert.Empty(t, counters)
}

func TestTailscaleScraperScrape(t *testing.T) {
	var inbound, outbound atomic.Int64
	inbound.Store(1000)
	outbound.Store(5000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `# TYPE tailscaled_serve_inbound_bytes_total counter
tailscaled_serve_inbound_bytes_total{service="svc:write"} %d
# TYPE tailscaled_serve_outbound_bytes_total counter
tailscaled_serve_outbound_bytes_total{service="svc:write"} %d
`, inbound.Load(), outbound.Load())
	}))
	defer server.Close()

	scraper := NewTailscaleScraper(TailscaleScraperSettings{
		URL:        server.URL,
		BufferSize: 10,
	})

	scraper.Scrape(context.Background())
	require.NoError(t, scraper.LastError())

	samples := scraper.Fetch("write", 1)
	require.Len(t, samples, 1)
	assert.Equal(t, int64(0), samples[0].InboundBytes)
	assert.Equal(t, int64(0), samples[0].OutboundBytes)

	inbound.Store(1500)
	outbound.Store(6500)
	scraper.Scrape(context.Background())

	samples = scraper.Fetch("write", 1)
	require.Len(t, samples, 1)
	assert.Equal(t, int64(500), samples[0].InboundBytes)
	assert.Equal(t, int64(1500), samples[0].OutboundBytes)
}

func TestTailscaleScraperDeltaCounterReset(t *testing.T) {
	scraper := NewTailscaleScraper(TailscaleScraperSettings{BufferSize: 10})

	scraper.recordSamples(map[string]*tailscaleCounters{
		"write": {inbound: 1000, outbound: 5000},
	})
	scraper.recordSamples(map[string]*tailscaleCounters{
		"write": {inbound: 10, outbound: 20},
	})

	samples := scraper.Fetch("write", 2)
	require.Len(t, samples, 2)

	// A restarted tailscaled resets the counters; the current values are
	// treated as the delta rather than producing a negative sample.
	assert.Equal(t, int64(10), samples[0].InboundBytes)
	assert.Equal(t, int64(20), samples[0].OutboundBytes)
}

func TestTailscaleScraperFetchUnknownService(t *testing.T) {
	scraper := NewTailscaleScraper(TailscaleScraperSettings{})

	scraper.recordSamples(map[string]*tailscaleCounters{
		"write": {inbound: 1000, outbound: 5000},
	})

	assert.Nil(t, scraper.Fetch("budget", 5))
}

func TestTailscaleScraperScrapeServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	scraper := NewTailscaleScraper(TailscaleScraperSettings{URL: server.URL})
	scraper.Scrape(context.Background())

	require.Error(t, scraper.LastError())
	assert.Contains(t, scraper.LastError().Error(), "unexpected status")
}

func TestTailscaleScraperSettingsDefaults(t *testing.T) {
	settings := TailscaleScraperSettings{}
	settings = settings.withDefaults()

	assert.Equal(t, defaultTailscaleMetricsURL, settings.URL)
	assert.Equal(t, defaultBufferSize, settings.BufferSize)
}
