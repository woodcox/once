package metrics

import (
	"context"
	"errors"
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

func TestTailscaleScraperSkipsOverlappingScrape(t *testing.T) {
	var requests atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		fmt.Fprintln(w, "")
	}))
	defer server.Close()

	scraper := NewTailscaleScraper(TailscaleScraperSettings{URL: server.URL})

	done := make(chan struct{})
	go func() {
		scraper.Scrape(context.Background())
		close(done)
	}()

	<-started

	// A second scrape while the first is still in flight must be skipped.
	scraper.Scrape(context.Background())

	close(release)
	<-done

	assert.Equal(t, int32(1), requests.Load())
}

func TestTailscaleScraperSkippedTicksAgeOutTraffic(t *testing.T) {
	// Pre-seed a tracked Service with nonzero traffic.
	scraper := NewTailscaleScraper(TailscaleScraperSettings{BufferSize: 3})
	scraper.recordSamples(map[string]*tailscaleCounters{
		"write": {inbound: 1000, outbound: 5000},
	})
	scraper.recordSamples(map[string]*tailscaleCounters{
		"write": {inbound: 2000, outbound: 6000},
	})

	// A scrape that blocks, so later Scrape calls are skipped as overlapping.
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		fmt.Fprintln(w, "")
	}))
	defer server.Close()
	scraper.settings.URL = server.URL

	done := make(chan struct{})
	go func() {
		scraper.Scrape(context.Background())
		close(done)
	}()

	<-started

	// Enough skipped ticks to age the nonzero samples out of the small ring.
	for range 3 {
		scraper.Scrape(context.Background())
	}

	close(release)
	<-done

	samples := scraper.Fetch("write", 3)
	require.Len(t, samples, 3)
	for _, s := range samples {
		assert.Zero(t, s.InboundBytes)
		assert.Zero(t, s.OutboundBytes)
	}
}

func TestTailscaleScraperExpiresAbsentService(t *testing.T) {
	scraper := NewTailscaleScraper(TailscaleScraperSettings{BufferSize: 10})

	scraper.recordSamples(map[string]*tailscaleCounters{
		"write": {inbound: 1000, outbound: 5000},
	})
	scraper.recordSamples(map[string]*tailscaleCounters{
		"write": {inbound: 1500, outbound: 6000},
	})

	// A successful scrape that no longer reports the Service records a zero
	// sample and drops the baseline.
	scraper.recordSamples(map[string]*tailscaleCounters{})

	samples := scraper.Fetch("write", 3)
	require.Len(t, samples, 3)
	assert.Zero(t, samples[0].InboundBytes)
	assert.Zero(t, samples[0].OutboundBytes)

	// A reappearing Service re-baselines instead of producing a stale delta.
	scraper.recordSamples(map[string]*tailscaleCounters{
		"write": {inbound: 1600, outbound: 6100},
	})
	latest := scraper.Fetch("write", 1)
	require.Len(t, latest, 1)
	assert.Zero(t, latest[0].InboundBytes)
	assert.Zero(t, latest[0].OutboundBytes)
}

func TestTailscaleScraperRebaselinesAfterFailure(t *testing.T) {
	var inbound, outbound atomic.Int64
	inbound.Store(1000)
	outbound.Store(5000)
	var failing atomic.Bool
	failing.Store(true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, `# TYPE tailscaled_serve_inbound_bytes_total counter
tailscaled_serve_inbound_bytes_total{service="svc:write"} %d
# TYPE tailscaled_serve_outbound_bytes_total counter
tailscaled_serve_outbound_bytes_total{service="svc:write"} %d
`, inbound.Load(), outbound.Load())
	}))
	defer server.Close()

	scraper := NewTailscaleScraper(TailscaleScraperSettings{URL: server.URL, BufferSize: 10})

	// Establish a baseline.
	failing.Store(false)
	scraper.Scrape(context.Background())
	require.NoError(t, scraper.LastError())

	// The counters jump while scraping fails.
	inbound.Store(5000)
	outbound.Store(20000)

	failing.Store(true)
	scraper.Scrape(context.Background())
	require.Error(t, scraper.LastError())

	// The first successful scrape after the outage re-baselines instead of
	// recording the whole jump as one interval.
	failing.Store(false)
	scraper.Scrape(context.Background())
	require.NoError(t, scraper.LastError())

	latest := scraper.Fetch("write", 1)
	require.Len(t, latest, 1)
	assert.Zero(t, latest[0].InboundBytes)
	assert.Zero(t, latest[0].OutboundBytes)
}

func TestTailscaleScraperAgesOutTrafficDuringOutage(t *testing.T) {
	// A small buffer keeps the test quick; the production scraper uses 200.
	scraper := NewTailscaleScraper(TailscaleScraperSettings{BufferSize: 5})

	// Build up nonzero traffic.
	scraper.recordSamples(map[string]*tailscaleCounters{
		"write": {inbound: 1000, outbound: 5000},
	})
	scraper.recordSamples(map[string]*tailscaleCounters{
		"write": {inbound: 2000, outbound: 6000},
	})

	// A long outage: more failed intervals than the buffer holds.
	for range 5 {
		scraper.fail(errors.New("outage"))
	}

	// The pre-outage traffic must have aged out of the ring.
	samples := scraper.Fetch("write", 5)
	require.Len(t, samples, 5)
	for _, s := range samples {
		assert.Zero(t, s.InboundBytes)
		assert.Zero(t, s.OutboundBytes)
	}
}
