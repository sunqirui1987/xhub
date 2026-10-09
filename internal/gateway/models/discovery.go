package models

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// catalogURLs 从首选目录生成最多两个同源候选，保留供应商自定义路径前缀和查询参数。
// 参数 rawURL：ModelsURL 生成的完整目录地址；返回：首选及可选 /v1 变体。
// 调用：fetchCatalog；非 HTTP 地址、带用户信息或缺少主机的地址返回错误，不发请求。
// 测试：discovery_test.go 的地址边界用例。
func catalogURLs(rawURL string) ([]string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return nil, errors.New("invalid model discovery URL")
	}
	urls := []string{u.String()}
	if !strings.HasSuffix(u.Path, "/models") {
		return urls, nil
	}
	root := strings.TrimSuffix(u.Path, "/models")
	if strings.HasSuffix(root, "/v1") {
		u.Path = strings.TrimSuffix(root, "/v1") + "/models"
	} else {
		u.Path = root + "/v1/models"
	}
	u.RawPath = ""
	urls = append(urls, u.String())
	return urls, nil
}

// fetchCatalog 按供应商首选地址拉取模型，仅在 404 时尝试另一种 /v1 目录路径。
// 参数 ctx：调用方取消信号；rawURL：首选目录；key：仅服务端使用的上游密钥。
// 返回：解析后的模型或错误；两次请求共用 20 秒超时，不写入凭据或模型。
// 调用：ListBuiltin；测试：discovery_test.go、regression/model_discovery_test.go。
func fetchCatalog(ctx context.Context, rawURL, key string) ([]CatalogModel, error) {
	urls, err := catalogURLs(rawURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// 回退只在当前供应商内进行；禁止重定向，避免把密钥转交到另一地址。
	client := *builtinClient
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	var attempts []string
	for _, candidate := range urls {
		items, status, err := fetchCatalogAttempt(ctx, &client, candidate, key)
		if err == nil {
			return items, nil
		}
		if status != http.StatusNotFound {
			return nil, err
		}
		u, _ := url.Parse(candidate)
		// 错误仅显示路径及状态，不包含可能携带秘密的查询参数或响应正文。
		attempts = append(attempts, u.EscapedPath()+": 404 Not Found")
	}
	return nil, fmt.Errorf("model discovery failed (%s)", strings.Join(attempts, "; "))
}

// fetchCatalogAttempt 执行一次服务端目录 GET 并校验 OpenAI 目录形状。
// 参数 ctx：共享超时；client：禁止重定向的客户端；rawURL、key：候选地址及凭据。
// 返回：模型、HTTP 状态和错误；网络及正文错误不触发路径回退，空模型列表视为成功。
// 调用：fetchCatalog；测试：discovery_test.go 的鉴权、无效响应及取消用例。
func fetchCatalogAttempt(ctx context.Context, client *http.Client, rawURL, key string) ([]CatalogModel, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, errors.New("invalid model discovery request")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("x-api-key", key)
	}
	resp, err := client.Do(req)
	if err != nil {
		// http.Client 的原始错误可能包含带查询参数的完整 URL，不能回传给浏览器。
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, errors.New("model discovery network request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, resp.StatusCode, errString(resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, errors.New("failed to read model discovery response")
	}
	if catalogObjects(body) == nil {
		return nil, resp.StatusCode, errors.New("invalid model discovery response: expected a model list")
	}
	return ParseCatalog(body), resp.StatusCode, nil
}
