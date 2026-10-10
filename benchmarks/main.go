// benchmark 通过真实 HTTP 压测平台，默认使用独立 schema 和本地上游。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/sunqirui1987/xhub/benchmarks/fixture"
	"github.com/sunqirui1987/xhub/benchmarks/load"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// report 保存运行参数和阶段结果，不包含密钥和数据库 URL。
type report struct {
	StartedAt, FinishedAt                                            time.Time
	Mode, GoVersion, Model, Schema, Duration, Timeout, UpstreamDelay string
	CPUs, Requests, Warmup, PromptBytes                              int
	CleanupOK, Cancelled, Passed                                     bool
	MaxErrorRate, MaxP95MS                                           float64
	Stages                                                           []load.Result
	Audit                                                            map[string]any
}

// main 处理信号与退出码；无参数/返回值，错误在资源清理之后输出。
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 解析 args、逐阶段发压并写报告；ctx 可取消，返回配置、运行或阈值错误。
// 所有返回路径清理隔离资源；已有网关模式只发流量，不创建或删除业务实体。
func run(ctx context.Context, args []string) (returnErr error) {
	flags := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	base := flags.String("base-url", "", "已有网关地址；留空启动隔离网关")
	model := flags.String("model", fixture.Model, "测试模型；已有网关需指定实际模型")
	scenarios := flags.String("scenarios", "chat,stream,models,auth-reject", "测试场景列表")
	levelsRaw := flags.String("concurrency", "1,4,16,64", "并发阶梯")
	requests := flags.Int("requests", 1000, "每场景每阶段请求上限")
	duration := flags.Duration("duration", 15*time.Second, "每阶段最长发压窗口")
	timeout := flags.Duration("timeout", 30*time.Second, "单请求超时")
	warmup := flags.Int("warmup", 10, "每场景每阶段预热数")
	prompt := flags.Int("prompt-bytes", 128, "唯一请求标记之外的正文长度")
	delay := flags.Duration("upstream-delay", 0, "本地上游延迟")
	maxError := flags.Float64("max-error-rate", 0.01, "允许错误率，范围 0..1")
	maxP95 := flags.Float64("max-p95-ms", 0, "P95 毫秒上限；0 不限制")
	output := flags.String("out", "", "报告目录；默认 benchmarks/reports/<UTC时间>-<PID>")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("不支持位置参数")
	}
	levels, err := parseLevels(*levelsRaw)
	if err != nil {
		return err
	}
	if *warmup < 0 || *warmup > 1000000 || *delay < 0 || *maxError < 0 || *maxError > 1 || *maxP95 < 0 || math.IsNaN(*maxError) || math.IsInf(*maxError, 0) || math.IsNaN(*maxP95) || math.IsInf(*maxP95, 0) {
		return errors.New("预热数、延迟或阈值无效")
	}
	selected := strings.Split(*scenarios, ",")
	for i := range selected {
		selected[i] = strings.TrimSpace(selected[i])
	}
	for _, scenario := range selected {
		c := load.Config{BaseURL: "http://127.0.0.1", Key: "validation", Model: *model, Scenario: scenario, Concurrency: levels[0], Requests: *requests, Duration: *duration, Timeout: *timeout, PromptBytes: *prompt}
		if *base != "" {
			c.BaseURL = *base
		}
		if err := c.Validate(); err != nil {
			return err
		}
	}
	r := report{StartedAt: time.Now().UTC(), Mode: "isolated", GoVersion: runtime.Version(), CPUs: runtime.NumCPU(), Model: *model, Requests: *requests, Duration: duration.String(), Timeout: timeout.String(), Warmup: *warmup, PromptBytes: *prompt, UpstreamDelay: delay.String(), Passed: true, MaxErrorRate: *maxError, MaxP95MS: *maxP95}
	dir := *output
	if dir == "" {
		dir = filepath.Join("benchmarks", "reports", fmt.Sprintf("%s-%d", r.StartedAt.Format("20060102-150405"), os.Getpid()))
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	var env *fixture.Environment
	key := os.Getenv("XHUB_BENCHMARK_KEY")
	if *base == "" {
		if *model != fixture.Model {
			return errors.New("隔离模式只支持 benchmark-chat 模型")
		}
		// 将网关逐请求诊断保存在报告目录，终端只展示阶段结果；保留日志写入成本。
		diagnostic, logErr := os.OpenFile(filepath.Join(dir, "diagnostic.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if logErr != nil {
			return logErr
		}
		previousLogOutput := log.Writer()
		log.SetOutput(diagnostic)
		defer func() { log.SetOutput(previousLogOutput); returnErr = errors.Join(returnErr, diagnostic.Close()) }()
		dsn := os.Getenv("XHUB_TEST_DATABASE_URL")
		if dsn == "" {
			dsn = fixture.DefaultDatabaseURL
		}
		env, err = fixture.Open(ctx, dsn, *delay)
		if err != nil {
			return err
		}
		*base, key, r.Schema = env.URL, env.Key, env.Schema
	} else {
		r.Mode = "existing-gateway"
		if key == "" {
			return errors.New("已有网关模式必须设置 XHUB_BENCHMARK_KEY")
		}
	}
	// 统一退出路径保留阈值失败与取消的证据，并用未取消上下文清理数据库。
	defer func() {
		if env != nil {
			r.Audit, err = env.Audit()
			returnErr = errors.Join(returnErr, err)
			if r.Audit != nil && r.Audit["consistent"] != true {
				returnErr = errors.Join(returnErr, errors.New("上游调用、token、用量或费用不一致"))
			}
			closeErr := env.Close()
			r.CleanupOK = closeErr == nil
			returnErr = errors.Join(returnErr, closeErr)
		}
		r.Cancelled = ctx.Err() != nil
		r.FinishedAt = time.Now().UTC()
		r.Passed = r.Passed && returnErr == nil && !r.Cancelled
		returnErr = errors.Join(returnErr, writeReport(dir, r))
		fmt.Printf("报告目录: %s\n", dir)
	}()
	for _, concurrency := range levels {
		for _, scenario := range selected {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c := load.Config{BaseURL: *base, Key: key, Model: *model, Scenario: scenario, Concurrency: concurrency, Requests: *requests, Duration: *duration, Timeout: *timeout, PromptBytes: *prompt}
			if *warmup > 0 {
				warm := c
				warm.Requests = *warmup
				result, err := load.Run(ctx, warm)
				if err != nil {
					return err
				}
				if result.Failed > 0 || result.Attempted == 0 {
					return errors.New("预热失败；请检查模型、API key 和网关状态")
				}
			}
			result, err := load.Run(ctx, c)
			if err != nil {
				return err
			}
			r.Stages = append(r.Stages, result)
			passed := result.Attempted > 0 && result.ErrorRate <= *maxError && (*maxP95 == 0 || result.Latency.P95 <= *maxP95)
			if !passed {
				r.Passed = false
			}
			fmt.Printf("%-11s concurrency=%-4d requests=%-6d ok=%-6d errors=%-5d rps=%8.2f p95=%8.2fms pass=%t\n", scenario, concurrency, result.Attempted, result.Succeeded, result.Failed, result.RPS, result.Latency.P95, passed)
		}
	}
	if !r.Passed {
		return errors.New("压测未达到错误率或 P95 阈值，详见报告")
	}
	return ctx.Err()
}

// parseLevels 解析并去重并发阶梯；raw 为逗号分隔文本，返回升序正整数或参数错误，无副作用。
func parseLevels(raw string) ([]int, error) {
	seen := map[int]bool{}
	for _, item := range strings.Split(raw, ",") {
		level, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || level < 1 || level > 4096 {
			return nil, errors.New("concurrency 必须是 1..4096 的逗号分隔整数")
		}
		seen[level] = true
	}
	levels := make([]int, 0, len(seen))
	for level := range seen {
		levels = append(levels, level)
	}
	sort.Ints(levels)
	return levels, nil
}

// writeReport 写 JSON 和中文 Markdown；参数为目录与报告，返回文件错误。
// 调用场景为 CLI 退出；只写统计和隔离 schema，不写凭据、URL 或请求正文。
func writeReport(dir string, r report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), append(data, '\n'), 0600); err != nil {
		return err
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# xhub 基准测试\n\n模式：%s；通过：%t；隔离清理完成：%t；取消：%t。\n\nGo：%s；CPU：%d；正文：%d bytes；上游延迟：%s。\n\n", r.Mode, r.Passed, r.CleanupOK, r.Cancelled, r.GoVersion, r.CPUs, r.PromptBytes, r.UpstreamDelay)
	md.WriteString("| 场景 | 并发 | 请求 | 成功 | 失败 | RPS | 成功 RPS | P50 ms | P95 ms | P99 ms | TTFT P95 ms |\n| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, s := range r.Stages {
		fmt.Fprintf(&md, "| %s | %d | %d | %d | %d | %.2f | %.2f | %.2f | %.2f | %.2f | %.2f |\n", s.Scenario, s.Concurrency, s.Attempted, s.Succeeded, s.Failed, s.RPS, s.SuccessRPS, s.Latency.P50, s.Latency.P95, s.Latency.P99, s.TTFT.P95)
	}
	if r.Audit != nil {
		audit, _ := json.Marshal(r.Audit)
		fmt.Fprintf(&md, "\n数据面审计：%s\n", audit)
	}
	md.WriteString("\n闭环并发压测；RPS 包含在途请求排空时间。预热不计入阶段统计，但计入隔离环境的用量审计。客户端、网关、上游共用主机，结果不能作为生产容量保证。\n")
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(md.String()), 0600)
}
