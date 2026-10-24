package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPluginRegister(t *testing.T) {
	raw, err := handleMethod("plugin.register", nil)
	if err != nil {
		t.Fatalf("handleMethod(plugin.register) err: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got ok=false")
	}

	var reg registration
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("unmarshal registration: %v", err)
	}

	if reg.Metadata.Name != pluginName {
		t.Errorf("expected plugin name %q, got %q", pluginName, reg.Metadata.Name)
	}
	if !reg.Capabilities.ManagementAPI {
		t.Errorf("expected management_api capability to be true")
	}
	if reg.SchemaVersion != 6 {
		t.Errorf("expected schema version 6, got %d", reg.SchemaVersion)
	}
}

func TestManagementRegister(t *testing.T) {
	raw, err := handleMethod("management.register", nil)
	if err != nil {
		t.Fatalf("handleMethod(management.register) err: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true")
	}

	var reg managementRegistration
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("unmarshal managementRegistration: %v", err)
	}

	// 检查资源页面
	hasSpeedtestResource := false
	for _, res := range reg.Resources {
		if res.Path == resourcePath && res.Menu == "模型测速" {
			hasSpeedtestResource = true
			break
		}
	}
	if !hasSpeedtestResource {
		t.Errorf("expected resource route %q with Menu '模型测速'", resourcePath)
	}

	// 检查管理 API 路由
	hasBatchRoute := false
	hasSingleRoute := false
	for _, route := range reg.Routes {
		if strings.Contains(route.Path, "batch-test") && route.Method == http.MethodPost {
			hasBatchRoute = true
		}
		if strings.Contains(route.Path, "test") && route.Method == http.MethodPost {
			hasSingleRoute = true
		}
	}
	if !hasBatchRoute {
		t.Errorf("expected POST batch-test route")
	}
	if !hasSingleRoute {
		t.Errorf("expected POST test route")
	}
}

func TestServeSpeedtestHTML(t *testing.T) {
	req := managementRequest{
		Method: http.MethodGet,
		Path:   "/v0/resource/plugins/cpa-models-speed-test/speedtest",
		Headers: http.Header{},
		Query:   url.Values{},
	}
	rawReq, _ := json.Marshal(req)

	raw, err := handleMethod("management.handle", rawReq)
	if err != nil {
		t.Fatalf("handleMethod(management.handle) err: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true")
	}

	var resp managementResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("unmarshal managementResponse: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if len(resp.Body) == 0 {
		t.Errorf("expected non-empty HTML body")
	}
	if !strings.Contains(string(resp.Body), "CPA") && !strings.Contains(string(resp.Body), "html") {
		t.Errorf("expected HTML body containing CPA, got %s", string(resp.Body[:100]))
	}
}

func TestIsQuotaError(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"HTTP 429 Too Many Requests", true},
		{"insufficient_quota: you exceeded your current quota", true},
		{"rate_limit_exceeded: Rate limit reached for requests", true},
		{"RESOURCE_EXHAUSTED: quota exceeded for model", true},
		{"connection reset by peer", false},
		{"context deadline exceeded", false},
		{"invalid api key provided", false},
	}

	for _, tc := range tests {
		got := isQuotaError(tc.input)
		if got != tc.expected {
			t.Errorf("isQuotaError(%q) = %v; want %v", tc.input, got, tc.expected)
		}
	}
}

func TestBatchSummaryCalculation(t *testing.T) {
	results := []TestResult{
		{Model: "m1", Status: "Success", TTFTMs: 200},
		{Model: "m2", Status: "Success", TTFTMs: 400},
		{Model: "m3", Status: "Quota Limit", TTFTMs: -1},
		{Model: "m4", Status: "Error", TTFTMs: -1},
	}

	var totalTTFT int64
	var successCount, quotaCount, errorCount int
	for _, r := range results {
		switch r.Status {
		case "Success":
			successCount++
			totalTTFT += r.TTFTMs
		case "Quota Limit":
			quotaCount++
		default:
			errorCount++
		}
	}
	avgTTFT := totalTTFT / int64(successCount)

	summary := BatchSummary{
		Total:      len(results),
		Success:    successCount,
		QuotaLimit: quotaCount,
		Error:      errorCount,
		AvgTTFTMs:  avgTTFT,
	}

	if summary.Total != 4 || summary.Success != 2 || summary.QuotaLimit != 1 || summary.Error != 1 {
		t.Errorf("unexpected counts: %+v", summary)
	}
	if summary.AvgTTFTMs != 300 {
		t.Errorf("expected avg TTFT 300ms, got %d", summary.AvgTTFTMs)
	}
}
