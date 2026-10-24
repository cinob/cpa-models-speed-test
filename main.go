package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}

static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) {
		return 1;
	}
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}

static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) {
		stored_host->free_buffer(ptr, len);
	}
}
*/
import "C"

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

//go:embed web/dist/index.html
var embeddedIndexHTML []byte

const (
	pluginName          = "cpa-models-speed-test"
	pluginVersion       = "1.0.0"
	resourcePath        = "/speedtest"
	resourceContentType = "text/html; charset=utf-8"
	defaultConcurrency  = 20
)

// RPC Envelope structures
type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginapi.Metadata       `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type registrationCapabilities struct {
	ManagementAPI bool `json:"management_api"`
}

type managementRegistration struct {
	Routes    []pluginapi.ManagementRoute `json:"routes,omitempty"`
	Resources []pluginapi.ResourceRoute   `json:"resources,omitempty"`
}

type managementRequest struct {
	Method         string      `json:"Method"`
	Path           string      `json:"Path"`
	Headers        http.Header `json:"Headers"`
	Query          url.Values  `json:"Query"`
	Body           []byte      `json:"Body"`
	HostCallbackID string      `json:"host_callback_id,omitempty"`
}

type managementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

type hostModelExecutionRequest struct {
	pluginapi.HostModelExecutionRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

// Model & Speed Test Target definitions
type ModelTarget struct {
	Key         string `json:"key"`
	ChannelID   string `json:"channel_id"`
	AuthIndex   string `json:"auth_index"`
	ChannelName string `json:"channel_name"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Disabled    bool   `json:"disabled"`
}

type ChannelInfo struct {
	ID        string   `json:"id"`
	AuthIndex string   `json:"auth_index"`
	Name      string   `json:"name"`
	Provider  string   `json:"provider"`
	Disabled  bool     `json:"disabled"`
	Models    []string `json:"models"`
}

type ChannelsResponse struct {
	OK       bool          `json:"ok"`
	Channels []ChannelInfo `json:"channels"`
	Models   []ModelTarget `json:"models"`
}

type TestResult struct {
	Key         string `json:"key"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	TTFTMs      int64  `json:"ttft_ms"`
	TotalMs     int64  `json:"total_ms"`
	Status      string `json:"status"` // "Success", "Error", "Quota Limit"
	StatusCode  int    `json:"status_code"`
	Message     string `json:"message"`
	TestedAt    string `json:"tested_at"`
}

type SingleTestResponse struct {
	OK     bool       `json:"ok"`
	Result TestResult `json:"result"`
}

type BatchTestRequest struct {
	Concurrency int           `json:"concurrency,omitempty"`
	Items       []ModelTarget `json:"items,omitempty"`
}

type BatchSummary struct {
	Total      int   `json:"total"`
	Success    int   `json:"success"`
	QuotaLimit int   `json:"quota_limit"`
	Error      int   `json:"error"`
	AvgTTFTMs  int64 `json:"avg_ttft_ms"`
}

type BatchTestResponse struct {
	OK      bool         `json:"ok"`
	Results []TestResult `json:"results"`
	Summary BatchSummary `json:"summary"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleMethod(C.GoString(method), requestBytes)
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
	_ = len
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegistration{
			Resources: []pluginapi.ResourceRoute{
				{
					Path:        resourcePath,
					Menu:        "模型测速",
					Description: "一键批量测试所有配置渠道的模型首字延迟 (TTFT) 与可用性",
				},
				{
					Path:        "/api/channels",
					Menu:        "",
					Description: "获取当前配置的所有上游渠道与模型列表 (GET)",
				},
				{
					Path:        "/api/test",
					Menu:        "",
					Description: "单独测速指定的渠道与模型 (GET)",
				},
				{
					Path:        "/api/batch-test",
					Menu:        "",
					Description: "批量并发测速模型 (GET)",
				},
			},
			Routes: []pluginapi.ManagementRoute{
				{
					Method: http.MethodGet,
					Path:   "/plugins/" + pluginName + "/channels",
				},
				{
					Method: http.MethodPost,
					Path:   "/plugins/" + pluginName + "/test",
				},
				{
					Method: http.MethodPost,
					Path:   "/plugins/" + pluginName + "/batch-test",
				},
			},
		})
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           "cinob",
			GitHubRepository: "https://github.com/router-for-me/cpa-models-speed-test",
			Logo:             "https://raw.githubusercontent.com/router-for-me/CLIProxyAPI/main/docs/logo.png",
			ConfigFields: []pluginapi.ConfigField{
				{
					Name:        "concurrency",
					Type:        "int",
					Default:     defaultConcurrency,
					Description: "并发测速 Goroutine 数量限制 (默认 20)",
				},
			},
		},
		Capabilities: registrationCapabilities{
			ManagementAPI: true,
		},
	}
}

func handleManagement(raw []byte) ([]byte, error) {
	var req managementRequest
	if len(raw) > 0 {
		if errUnmarshal := json.Unmarshal(raw, &req); errUnmarshal != nil {
			return nil, fmt.Errorf("decode management request: %w", errUnmarshal)
		}
	}

	path := strings.TrimSpace(req.Path)
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	// 1. 静态资源路由: /v0/resource/plugins/cpa-models-speed-test/speedtest
	if strings.HasSuffix(path, resourcePath) || strings.HasSuffix(path, resourcePath+"/") {
		return okEnvelope(managementResponse{
			StatusCode: http.StatusOK,
			Headers: http.Header{
				"Content-Type":  []string{resourceContentType},
				"Cache-Control": []string{"no-cache"},
			},
			Body: getIndexHTML(),
		})
	}

	// 2. 渠道与模型列表: GET /channels 或 /api/channels
	if strings.HasSuffix(path, "/channels") {
		channelsResp, err := fetchChannelsAndModels()
		if err != nil {
			return okEnvelope(jsonResponse(http.StatusInternalServerError, map[string]any{
				"ok":    false,
				"error": err.Error(),
			}))
		}
		return okEnvelope(jsonResponse(http.StatusOK, channelsResp))
	}

	// 3. 单独测速: POST /test 或 GET /api/test?channel_id=...&model=...&provider=...
	if strings.HasSuffix(path, "/test") {
		var target ModelTarget
		if method == http.MethodPost && len(req.Body) > 0 {
			if err := json.Unmarshal(req.Body, &target); err != nil {
				return okEnvelope(jsonResponse(http.StatusBadRequest, map[string]any{
					"ok":    false,
					"error": "invalid json body: " + err.Error(),
				}))
			}
		} else {
			target.ChannelID = strings.TrimSpace(req.Query.Get("channel_id"))
			target.Provider = strings.TrimSpace(req.Query.Get("provider"))
			target.Model = strings.TrimSpace(req.Query.Get("model"))
		}

		if target.Model == "" {
			return okEnvelope(jsonResponse(http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": "model is required for speed test",
			}))
		}

		result := probeSingleModel(target, req.HostCallbackID)
		return okEnvelope(jsonResponse(http.StatusOK, SingleTestResponse{
			OK:     true,
			Result: result,
		}))
	}

	// 4. 一键批量并发测速: POST /batch-test 或 GET /api/batch-test
	if strings.HasSuffix(path, "/batch-test") {
		var batchReq BatchTestRequest
		if method == http.MethodPost && len(req.Body) > 0 {
			_ = json.Unmarshal(req.Body, &batchReq)
		} else {
			if rawConcurrency := req.Query.Get("concurrency"); rawConcurrency != "" {
				if c, err := strconv.Atoi(rawConcurrency); err == nil {
					batchReq.Concurrency = c
				}
			}
		}

		// 若未指定 items，则自动加载所有可用模型
		if len(batchReq.Items) == 0 {
			channelsResp, err := fetchChannelsAndModels()
			if err == nil {
				batchReq.Items = channelsResp.Models
			}
		}

		results, summary := runBatchSpeedTest(batchReq.Items, batchReq.Concurrency, req.HostCallbackID)
		return okEnvelope(jsonResponse(http.StatusOK, BatchTestResponse{
			OK:      true,
			Results: results,
			Summary: summary,
		}))
	}

	return okEnvelope(jsonResponse(http.StatusNotFound, map[string]any{
		"ok":    false,
		"error": "not found: " + path,
	}))
}

// fetchChannelsAndModels 从宿主回调 host.auth.models 与 host.auth.list 获取当前配置的所有上游渠道账号及可用模型列表
func fetchChannelsAndModels() (ChannelsResponse, error) {
	// 1. 获取凭证的注册模型: host.auth.models
	rawModels, errModels := callHost(pluginabi.MethodHostAuthModels, pluginapi.HostAuthModelsRequest{})
	if errModels != nil {
		return ChannelsResponse{}, fmt.Errorf("call host.auth.models: %w", errModels)
	}

	var authModelsResp pluginapi.HostAuthModelsResponse
	if err := json.Unmarshal(rawModels, &authModelsResp); err != nil {
		return ChannelsResponse{}, fmt.Errorf("decode host.auth.models response: %w", err)
	}

	// 2. 获取凭证元信息 (标签、名称、禁用状态等): host.auth.list
	type rpcAuthListResponse struct {
		Files []pluginapi.HostAuthFileEntry `json:"files"`
	}
	fileMap := make(map[string]pluginapi.HostAuthFileEntry)
	rawFiles, errFiles := callHost(pluginabi.MethodHostAuthList, map[string]any{})
	if errFiles == nil {
		var listResp rpcAuthListResponse
		if err := json.Unmarshal(rawFiles, &listResp); err == nil {
			for _, f := range listResp.Files {
				if f.ID != "" {
					fileMap[f.ID] = f
				}
				if f.AuthIndex != "" {
					fileMap[f.AuthIndex] = f
				}
			}
		}
	}

	channels := make([]ChannelInfo, 0, len(authModelsResp.Credentials))
	models := make([]ModelTarget, 0)

	for _, cred := range authModelsResp.Credentials {
		fileInfo, hasFile := fileMap[cred.ID]
		if !hasFile {
			fileInfo = fileMap[cred.AuthIndex]
		}

		name := cred.ID
		disabled := false
		if fileInfo.Label != "" {
			name = fileInfo.Label
		} else if fileInfo.Name != "" {
			name = fileInfo.Name
		}
		if fileInfo.Disabled || fileInfo.Unavailable {
			disabled = true
		}

		provider := cred.Provider
		if provider == "" && fileInfo.Provider != "" {
			provider = fileInfo.Provider
		}

		channels = append(channels, ChannelInfo{
			ID:        cred.ID,
			AuthIndex: cred.AuthIndex,
			Name:      name,
			Provider:  provider,
			Disabled:  disabled,
			Models:    cred.Models,
		})

		for _, m := range cred.Models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			models = append(models, ModelTarget{
				Key:         cred.ID + "::" + m,
				ChannelID:   cred.ID,
				AuthIndex:   cred.AuthIndex,
				ChannelName: name,
				Provider:    provider,
				Model:       m,
				Disabled:    disabled,
			})
		}
	}

	return ChannelsResponse{
		OK:       true,
		Channels: channels,
		Models:   models,
	}, nil
}

// probeSingleModel 后端代发流式轻量探测：发送最简 Prompt，收到首个 Token 时立即主动断开连接计算精确 TTFT
func probeSingleModel(target ModelTarget, hostCallbackID string) TestResult {
	start := time.Now()
	nowStr := time.Now().Format("15:04:05")

	res := TestResult{
		Key:         target.Key,
		ChannelID:   target.ChannelID,
		ChannelName: target.ChannelName,
		Provider:    target.Provider,
		Model:       target.Model,
		TTFTMs:      -1,
		TotalMs:     0,
		Status:      "Error",
		StatusCode:  0,
		Message:     "",
		TestedAt:    nowStr,
	}

	if res.Key == "" {
		res.Key = target.ChannelID + "::" + target.Model
	}

	if target.Disabled {
		res.Status = "Disabled"
		res.Message = "模型已禁用"
		return res
	}

	// 极简 Prompt，请求仅生成 1 个 Token，stream: true
	body, errBody := json.Marshal(map[string]any{
		"model":      target.Model,
		"stream":     true,
		"max_tokens": 1,
		"messages": []map[string]string{
			{"role": "user", "content": "1"},
		},
	})
	if errBody != nil {
		res.Message = "marshal body failed: " + errBody.Error()
		res.TotalMs = time.Since(start).Milliseconds()
		return res
	}

	// 构造宿主模型流式执行请求，通过 AuthID 锁定具体渠道凭证
	execReq := hostModelExecutionRequest{
		HostModelExecutionRequest: pluginapi.HostModelExecutionRequest{
			EntryProtocol:  "openai",
			ExitProtocol:   "openai",
			Model:          target.Model,
			Stream:         true,
			Body:           body,
			AuthID:         target.ChannelID,
			ForcedProvider: target.Provider,
		},
		HostCallbackID: hostCallbackID,
	}

	rawStreamResp, errCall := callHost(pluginabi.MethodHostModelExecuteStream, execReq)
	if errCall != nil {
		res.TotalMs = time.Since(start).Milliseconds()
		res.Message = errCall.Error()
		if isQuotaError(res.Message) {
			res.Status = "Quota Limit"
			res.StatusCode = http.StatusTooManyRequests
		} else {
			res.Status = "Error"
		}
		return res
	}

	var streamResp pluginapi.HostModelStreamResponse
	if err := json.Unmarshal(rawStreamResp, &streamResp); err != nil {
		res.TotalMs = time.Since(start).Milliseconds()
		res.Message = "decode stream response: " + err.Error()
		return res
	}

	res.StatusCode = streamResp.StatusCode

	// 上游非 200 响应判断
	if streamResp.StatusCode != http.StatusOK {
		if streamResp.StreamID != "" {
			_ = closeHostModelStream(streamResp.StreamID)
		}
		res.TotalMs = time.Since(start).Milliseconds()
		if streamResp.StatusCode == http.StatusTooManyRequests {
			res.Status = "Quota Limit"
			res.Message = "HTTP 429 Too Many Requests (Rate limit / Quota exceeded)"
		} else {
			res.Status = "Error"
			res.Message = fmt.Sprintf("HTTP status %d", streamResp.StatusCode)
		}
		return res
	}

	if streamResp.StreamID == "" {
		res.TotalMs = time.Since(start).Milliseconds()
		res.Message = "host returned empty stream_id"
		return res
	}

	// 收到官方流式返回后，读取第一个 Token/数据块立即记录 TTFT 并主动断开连接
	chunk, errRead := readHostModelStream(streamResp.StreamID)
	ttft := time.Since(start).Milliseconds()

	// 无论读取结果如何，立即主动关闭流，不浪费额度和下游 Token
	_ = closeHostModelStream(streamResp.StreamID)

	res.TotalMs = ttft

	if errRead != nil {
		res.Message = errRead.Error()
		if isQuotaError(res.Message) {
			res.Status = "Quota Limit"
			res.StatusCode = http.StatusTooManyRequests
		} else {
			res.Status = "Error"
		}
		return res
	}

	if chunk.Error != "" {
		res.Message = chunk.Error
		if isQuotaError(chunk.Error) {
			res.Status = "Quota Limit"
			res.StatusCode = http.StatusTooManyRequests
		} else {
			res.Status = "Error"
		}
		return res
	}

	// 正常收到首字 (First Token)
	res.TTFTMs = ttft
	res.Status = "Success"
	res.Message = "OK"
	return res
}

// runBatchSpeedTest 使用 Goroutine 并发批量测试所有模型
func runBatchSpeedTest(items []ModelTarget, concurrency int, hostCallbackID string) ([]TestResult, BatchSummary) {
	if concurrency <= 0 || concurrency > 50 {
		concurrency = defaultConcurrency
	}

	results := make([]TestResult, len(items))
	if len(items) == 0 {
		return results, BatchSummary{}
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		go func(idx int, target ModelTarget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = probeSingleModel(target, hostCallbackID)
		}(i, item)
	}

	wg.Wait()

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

	var avgTTFT int64
	if successCount > 0 {
		avgTTFT = totalTTFT / int64(successCount)
	}

	summary := BatchSummary{
		Total:      len(items),
		Success:    successCount,
		QuotaLimit: quotaCount,
		Error:      errorCount,
		AvgTTFTMs:  avgTTFT,
	}

	return results, summary
}

func readHostModelStream(streamID string) (pluginapi.HostModelStreamReadResponse, error) {
	result, errCall := callHost(pluginabi.MethodHostModelStreamRead, pluginapi.HostModelStreamReadRequest{StreamID: streamID})
	if errCall != nil {
		return pluginapi.HostModelStreamReadResponse{}, errCall
	}
	var resp pluginapi.HostModelStreamReadResponse
	if errUnmarshal := json.Unmarshal(result, &resp); errUnmarshal != nil {
		return pluginapi.HostModelStreamReadResponse{}, fmt.Errorf("decode host.model.stream_read result: %w", errUnmarshal)
	}
	return resp, nil
}

func closeHostModelStream(streamID string) error {
	_, errCall := callHost(pluginabi.MethodHostModelStreamClose, pluginapi.HostModelStreamCloseRequest{StreamID: streamID})
	return errCall
}

func isQuotaError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "429") ||
		strings.Contains(lower, "quota") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "ratelimit") ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "insufficient") ||
		strings.Contains(lower, "credit") ||
		strings.Contains(lower, "billing") ||
		strings.Contains(lower, "exceeded") ||
		strings.Contains(lower, "exhausted") ||
		strings.Contains(lower, "resource_exhausted")
}

func getIndexHTML() []byte {
	if len(embeddedIndexHTML) > 0 {
		return embeddedIndexHTML
	}
	return []byte("<!doctype html><html><body><h3>CPA 模型测速面板正在加载...</h3><p>请先在 web/ 目录执行 bun run build 生成前端资产。</p></body></html>")
}

func jsonResponse(statusCode int, data any) managementResponse {
	body, _ := json.Marshal(data)
	return managementResponse{
		StatusCode: statusCode,
		Headers: http.Header{
			"Content-Type": []string{"application/json; charset=utf-8"},
		},
		Body: body,
	}
}

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, fmt.Errorf("marshal host payload %s: %w", method, errMarshal)
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))

	var response C.cliproxy_buffer
	var requestPtr *C.uint8_t
	if len(rawPayload) > 0 {
		cPayload := C.CBytes(rawPayload)
		if cPayload == nil {
			return nil, fmt.Errorf("allocate host payload %s", method)
		}
		defer C.free(cPayload)
		requestPtr = (*C.uint8_t)(cPayload)
	}
	callCode := C.call_host_api(cMethod, requestPtr, C.size_t(len(rawPayload)), &response)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
	}
	if response.ptr != nil {
		C.free_host_buffer(response.ptr, response.len)
	}
	if len(rawResponse) == 0 {
		return nil, fmt.Errorf("host callback %s returned no response, code=%d", method, int(callCode))
	}

	var env envelope
	if errUnmarshal := json.Unmarshal(rawResponse, &env); errUnmarshal != nil {
		return nil, fmt.Errorf("decode host envelope %s: %w", method, errUnmarshal)
	}
	if !env.OK {
		if env.Error != nil {
			return nil, fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
		}
		return nil, fmt.Errorf("host callback %s failed", method)
	}
	if callCode != 0 {
		return nil, fmt.Errorf("host callback %s returned code=%d", method, int(callCode))
	}
	return append(json.RawMessage(nil), env.Result...), nil
}

func okEnvelope(v any) ([]byte, error) {
	raw, errMarshal := json.Marshal(v)
	if errMarshal != nil {
		return nil, errMarshal
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
