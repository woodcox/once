package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	prom "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

const (
	defaultTailscaleMetricsURL = "http://100.100.100.100/metrics"

	tailscaleInboundMetric  = "tailscaled_serve_inbound_bytes_total"
	tailscaleOutboundMetric = "tailscaled_serve_outbound_bytes_total"
)

// TailscaleSample represents Serve byte deltas for a single scrape interval.
type TailscaleSample struct {
	Timestamp     time.Time
	InboundBytes  int64
	OutboundBytes int64
}

// TailscaleScraperSettings configures the Tailscale metrics scraper.
type TailscaleScraperSettings struct {
	URL        string
	BufferSize int
}

func (s TailscaleScraperSettings) withDefaults() TailscaleScraperSettings {
	if s.URL == "" {
		s.URL = defaultTailscaleMetricsURL
	}
	if s.BufferSize == 0 {
		s.BufferSize = defaultBufferSize
	}
	return s
}

// TailscaleScraper periodically reads per-Service byte counters from the local
// Tailscale client. These are the only Serve traffic metrics Tailscale exposes;
// it does not report request counts or unique visitors.
type TailscaleScraper struct {
	settings TailscaleScraperSettings
	client   *http.Client

	mu        sync.RWMutex
	services  map[string]*tailscaleServiceData
	lastError error
}

type tailscaleServiceData struct {
	samples      []TailscaleSample
	head         int
	count        int
	prevCounters *tailscaleCounters
}

type tailscaleCounters struct {
	inbound  float64
	outbound float64
}

func NewTailscaleScraper(settings TailscaleScraperSettings) *TailscaleScraper {
	settings = settings.withDefaults()
	return &TailscaleScraper{
		settings: settings,
		client:   &http.Client{Timeout: httpTimeout},
		services: make(map[string]*tailscaleServiceData),
	}
}

// Fetch returns the last n samples for a Service, ordered from newest to oldest.
// The Service name is the Tailscale Service name without the "svc:" prefix.
func (s *TailscaleScraper) Fetch(service string, n int) []TailscaleSample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.services[service]
	if !ok {
		return nil
	}

	available := min(n, data.count)
	result := make([]TailscaleSample, available)
	for i := range available {
		idx := (data.head - 1 - i + len(data.samples)) % len(data.samples)
		result[i] = data.samples[idx]
	}

	return result
}

func (s *TailscaleScraper) LastError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

func (s *TailscaleScraper) Scrape(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.settings.URL, nil)
	if err != nil {
		s.setError(fmt.Errorf("creating request: %w", err))
		return
	}

	resp, err := s.client.Do(req)
	if err != nil {
		s.setError(fmt.Errorf("fetching metrics: %w", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.setError(fmt.Errorf("fetching metrics: unexpected status %s", resp.Status))
		return
	}

	counters, err := s.parseMetrics(resp.Body)
	if err != nil {
		s.setError(fmt.Errorf("parsing metrics: %w", err))
		return
	}

	s.setError(nil)
	s.recordSamples(counters)
}

// Private

func (s *TailscaleScraper) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = err
}

func (s *TailscaleScraper) parseMetrics(body io.Reader) (map[string]*tailscaleCounters, error) {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(body)
	if err != nil {
		return nil, err
	}

	counters := make(map[string]*tailscaleCounters)

	add := func(family *prom.MetricFamily, inbound bool) {
		if family == nil {
			return
		}
		for _, metric := range family.GetMetric() {
			service := getLabel(metric, "service")
			if service == "" {
				continue
			}
			service = strings.TrimPrefix(service, "svc:")

			state, ok := counters[service]
			if !ok {
				state = &tailscaleCounters{}
				counters[service] = state
			}
			if inbound {
				state.inbound += metric.GetCounter().GetValue()
			} else {
				state.outbound += metric.GetCounter().GetValue()
			}
		}
	}

	add(families[tailscaleInboundMetric], true)
	add(families[tailscaleOutboundMetric], false)

	return counters, nil
}

func (s *TailscaleScraper) recordSamples(counters map[string]*tailscaleCounters) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	for service, current := range counters {
		data, ok := s.services[service]
		if !ok {
			data = &tailscaleServiceData{
				samples: make([]TailscaleSample, s.settings.BufferSize),
			}
			s.services[service] = data
		}

		sample := TailscaleSample{Timestamp: now}
		if data.prevCounters != nil {
			sample.InboundBytes = safeDelta(current.inbound, data.prevCounters.inbound)
			sample.OutboundBytes = safeDelta(current.outbound, data.prevCounters.outbound)
		}
		data.prevCounters = current

		data.samples[data.head] = sample
		data.head = (data.head + 1) % len(data.samples)
		if data.count < len(data.samples) {
			data.count++
		}
	}
}
