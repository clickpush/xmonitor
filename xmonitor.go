package xmonitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	LocalDestination = "http://localhost:8080/metrics/create"
	ProdDestination  = "https://xmonitor.clickpush.xyz/metrics/create"
)

type Config struct {
	// ...
	SendMetricsEnabled bool
	Destination        string
	LogingEnabled      bool
}

type XMonitor struct {
	cfg Config
}

func New(cfg Config) *XMonitor {
	return &XMonitor{cfg}
}

func (x *XMonitor) sendMetric(r *http.Request, statusCode int, latency *time.Duration) error {
	// send the metric
	data := map[string]interface{}{
		// request data
		"request": map[string]interface{}{
			"method": r.Method,
			"uri":    r.RequestURI,
			"host":   r.Host,
		},
		// response data
		"response": map[string]interface{}{
			"status": statusCode,
		},
	}

	if latency != nil {
		data["response"].(map[string]interface{})["latency"] = latency
	}

	byt, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.cfg.Destination, bytes.NewBuffer(byt))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", "some-token"))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send metric: %w", err)
	}

	if res.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to send metric: %s", res.Status)
	}

	return nil
}

func (x *XMonitor) Monitor(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		xw := x.NewMonitoredResponseWriter(r, w)
		h(xw, r)
	}
}

func (x *XMonitor) MonitorHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xw := x.NewMonitoredResponseWriter(r, w)
		h.ServeHTTP(xw, r)
	})
}

func (x *XMonitor) SendMetric(r *http.Request, statusCode int, latency *time.Duration) error {
	return x.sendMetric(r, statusCode, latency)
}

type MonitoredResponseWriter struct {
	x             *XMonitor
	r             *http.Request
	w             http.ResponseWriter
	tStart        *time.Time
	headerWritten bool
}

func (x *XMonitor) NewMonitoredResponseWriter(r *http.Request, w http.ResponseWriter) *MonitoredResponseWriter {
	tStart := time.Now()
	return &MonitoredResponseWriter{x: x, r: r, w: w, tStart: &tStart}
}

func (m *MonitoredResponseWriter) WriteHeader(statusCode int) {
	go func() {
		var latency *time.Duration
		if m.tStart != nil {
			l := time.Since(*m.tStart)
			latency = &l
		}
		err := m.x.sendMetric(m.r, statusCode, latency)
		if err != nil && m.x.cfg.LogingEnabled {
			fmt.Println("failed to send metric:", err)
		}
	}()

	m.w.WriteHeader(statusCode)
	m.headerWritten = true
}

func (m *MonitoredResponseWriter) Write(b []byte) (int, error) {
	if !m.headerWritten {
		m.WriteHeader(http.StatusOK)
	}
	return m.w.Write(b)
}

func (m *MonitoredResponseWriter) Header() http.Header {
	return m.w.Header()
}
