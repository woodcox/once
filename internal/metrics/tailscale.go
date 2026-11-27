package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

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

	scrapeMu  sync.Mutex
	mu        sync.RWMutex
	services  map[string]*tailscaleServiceData
	lastError error
}

type tailscaleServiceData struct {
	samples      sampleRing[TailscaleSample]
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

	return data.samples.lastN(n)
}

func (s *TailscaleScraper) LastError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

func (s *TailscaleScraper) Scrape(ctx context.Context) {
	if !s.scrapeMu.TryLock() {
		return // a scrape is already in flight; skip this tick
	}
	defer s.scrapeMu.Unlock()

	body, err := httpGet(ctx, s.client, s.settings.URL)
	if err != nil {
		s.fail(err)
		return
	}
	defer body.Close()

	counters, err := s.parseMetrics(body)
	if err != nil {
		s.fail(fmt.Errorf("parsing metrics: %w", err))
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

// fail records a scrape error and drops every Service's counter baseline, so a
// later successful scrape re-baselines instead of attributing the whole outage
// to a single interval.
func (s *TailscaleScraper) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = err
	for _, data := range s.services {
		data.prevCounters = nil
	}
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

	for service, current := range counters {
		data := s.serviceData(service)

		var sample TailscaleSample
		if data.prevCounters != nil {
			sample.InboundBytes = safeDelta(current.inbound, data.prevCounters.inbound)
			sample.OutboundBytes = safeDelta(current.outbound, data.prevCounters.outbound)
		}
		data.prevCounters = current

		data.samples.push(sample)
	}

	// Expire Services that were tracked before but are absent from this
	// successful response: record a zero sample and drop the baseline so a
	// reappearing Service re-baselines instead of producing a stale delta.
	for service, data := range s.services {
		if _, present := counters[service]; present {
			continue
		}
		data.prevCounters = nil
		data.samples.push(TailscaleSample{})
	}
}

func (s *TailscaleScraper) serviceData(service string) *tailscaleServiceData {
	data, ok := s.services[service]
	if !ok {
		data = &tailscaleServiceData{
			samples: newSampleRing[TailscaleSample](s.settings.BufferSize),
		}
		s.services[service] = data
	}
	return data
}
