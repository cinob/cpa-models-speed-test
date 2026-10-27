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
	if reg.Metadata.Version != pluginVersion {
		t.Errorf("expected plugin version %q, got %q", pluginVersion, reg.Metadata.Version)
	}
	if len(reg.Metadata.ConfigFields) < 2 {
		t.Errorf("expected at least 2 config fields, got %d", len(reg.Metadata.ConfigFields))
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

	// 检查资源页面：按安全规范，只允许注册静态资源页面，严禁注册动态 API
	if len(reg.Resources) != 1 {
		t.Fatalf("expected exactly 1 resource route, got %d", len(reg.Resources))
	}
	if reg.Resources[0].Path != resourcePath || reg.Resources[0].Menu != "模型测速" {
		t.Errorf("expected resource route %q with Menu '模型测速', got %+v", resourcePath, reg.Resources[0])
	}

	// 检查管理 API 路由
	hasBatchRoute := false
	hasSingleRoute := false
	hasChannelsRoute := false
	for _, route := range reg.Routes {
		if strings.Contains(route.Path, "batch-test") && route.Method == http.MethodPost {
			hasBatchRoute = true
		}
		if strings.HasSuffix(route.Path, "/test") && route.Method == http.MethodPost {
			hasSingleRoute = true
		}
		if strings.Contains(route.Path, "channels") && route.Method == http.MethodGet {
			hasChannelsRoute = true
		}
	}
	if !hasBatchRoute {
		t.Errorf("expected POST batch-test route")
	}
	if !hasSingleRoute {
		t.Errorf("expected POST test route")
	}
	if !hasChannelsRoute {
		t.Errorf("expected GET channels route")
	}
}

func TestResourceRouteRejectsDynamicAPIs(t *testing.T) {
	rejectedPaths := []string{
		"/v0/resource/plugins/cpa-models-speed-test/api/channels",
		"/v0/resource/plugins/cpa-models-speed-test/channels",
		"/v0/resource/plugins/cpa-models-speed-test/api/test",
		"/v0/resource/plugins/cpa-models-speed-test/test",
		"/v0/resource/plugins/cpa-models-speed-test/api/batch-test",
		"/v0/resource/plugins/cpa-models-speed-test/batch-test",
	}

	for _, p := range rejectedPaths {
		req := managementRequest{
			Method:  http.MethodGet,
			Path:    p,
			Headers: http.Header{},
			Query:   url.Values{},
		}
		rawReq, _ := json.Marshal(req)

		raw, err := handleMethod("management.handle", rawReq)
		if err != nil {
			t.Fatalf("handleMethod error on %s: %v", p, err)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		var resp managementResponse
		if err := json.Unmarshal(env.Result, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("path %s should be rejected on resource route with 404, got %d", p, resp.StatusCode)
		}
	}
}

func TestManagementRouteEnforcesMethod(t *testing.T) {
	// GET on POST-only /test route must return 404
	req := managementRequest{
		Method:  http.MethodGet,
		Path:    "/v0/management/plugins/cpa-models-speed-test/test",
		Headers: http.Header{},
		Query:   url.Values{},
	}
	rawReq, _ := json.Marshal(req)

	raw, err := handleMethod("management.handle", rawReq)
	if err != nil {
		t.Fatalf("handleMethod error: %v", err)
	}

	var env envelope
	_ = json.Unmarshal(raw, &env)
	var resp managementResponse
	_ = json.Unmarshal(env.Result, &resp)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /test should return 404, got %d", resp.StatusCode)
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

func TestProbeSingleModelDisabled(t *testing.T) {
	target := ModelTarget{
		ChannelID: "test-channel",
		Model:     "test-model",
		Disabled:  true,
	}
	res := probeSingleModel(target, "", 5)
	if res.Status != "Disabled" {
		t.Errorf("expected status 'Disabled', got %q", res.Status)
	}
	if res.Message != "模型已禁用" {
		t.Errorf("expected message '模型已禁用', got %q", res.Message)
	}
}
