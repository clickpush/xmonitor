package xmonitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	LocalDestination       = "http://localhost:8080/metrics/create"
	DockerLocalDestination = "http://xmonitor:8080/metric/create"
	ProdDestination        = "https://xmonitor.clickpush.xyz/metric/create"
)

type Config struct {
	// ...
	Destination   string
	LogingEnabled bool
}

type XMonitor struct {
	cfg Config
}

func New(cfg Config) *XMonitor {
	return &XMonitor{cfg}
}

func (x *XMonitor) sendMetric(r *http.Request, statusCode int) error {
	// send the metric
	data := map[string]interface{}{
		// request data
		"request": map[string]interface{}{
			"method": r.Method,
			"uri":    r.RequestURI,
		},
		// response data
		"response": map[string]interface{}{
			"status": statusCode,
		},
	}

	byt, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, x.cfg.Destination, bytes.NewBuffer(byt))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", "some-token"))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send metric: %w", err)
	}

	if res.StatusCode != http.StatusOK {
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

func (x *XMonitor) SendMetric(r *http.Request, statusCode int) error {
	return x.sendMetric(r, statusCode)
}

type MonitoredResponseWriter struct {
	x             *XMonitor
	r             *http.Request
	w             http.ResponseWriter
	headerWritten bool
}

func (x *XMonitor) NewMonitoredResponseWriter(r *http.Request, w http.ResponseWriter) *MonitoredResponseWriter {
	return &MonitoredResponseWriter{x: x, r: r, w: w}
}

func (m *MonitoredResponseWriter) WriteHeader(statusCode int) {
	err := m.x.sendMetric(m.r, statusCode)
	if err != nil && m.x.cfg.LogingEnabled {
		fmt.Println("failed to send metric:", err)
	}

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

