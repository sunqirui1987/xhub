package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestParseLevels 验证并发阶梯排序去重及上下界；固定文本输入，断言结果或错误，无需清理。
func TestParseLevels(t *testing.T) {
	got, err := parseLevels(" 16,1,4,4,4096 ")
	if err != nil || !reflect.DeepEqual(got, []int{1, 4, 16, 4096}) {
		t.Fatalf("并发阶梯错误: %v %v", got, err)
	}
	for _, raw := range []string{"", "0", "4097", "1,", "NaN", "-1"} {
		if _, err := parseLevels(raw); err == nil {
			t.Fatalf("未拒绝阶梯 %q", raw)
		}
	}
}

// TestIsolatedRegression 验证 CLI 默认隔离链路的报告、诊断日志、真实费用审计和清理；前置本地 PostgreSQL。
// 参数 t 为测试上下文；运行三请求四场景后断言结果，schema 由 CLI 删除、文件由 TempDir 清理。
func TestIsolatedRegression(t *testing.T) {
	dir := t.TempDir()
	if err := run(context.Background(), []string{"-requests", "3", "-concurrency", "2", "-warmup", "0", "-out", dir}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r report
	if err = json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	if !r.Passed || !r.CleanupOK || len(r.Stages) != 4 || r.Audit["consistent"] != true || r.Audit["upstream_calls"] != float64(6) {
		t.Fatalf("隔离 CLI 回归: %+v", r)
	}
	info, err := os.Stat(filepath.Join(dir, "diagnostic.log"))
	if err != nil || info.Size() == 0 {
		t.Fatalf("没有保存网关诊断: %v", err)
	}
}

// TestRunInvalidArguments 验证参数在联网前拒绝；无数据库前置，含非有限阈值，临时目录自动回收。
func TestRunInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"unexpected"}, {"-concurrency", "0"}, {"-warmup", "-1"}, {"-max-error-rate", "NaN"}, {"-max-p95-ms", "+Inf"}, {"-scenarios", "unknown"}, {"-requests", "0"}, {"-base-url", "http://user:secret@localhost"}} {
		if err := run(context.Background(), args); err == nil {
			t.Fatalf("未拒绝参数 %v", args)
		}
	}
	if err := run(context.Background(), []string{"-help"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XHUB_BENCHMARK_KEY", "")
	if err := run(context.Background(), []string{"-base-url", "http://127.0.0.1:1", "-out", t.TempDir()}); err == nil {
		t.Fatal("没有密钥仍允许已有网关模式")
	}
}

// TestRunReportAndFailure 验证 CLI 成功、阈值失败和取消均写报告；本地 HTTP 前置，不联网供应商，临时文件自动清理。
func TestRunReportAndFailure(t *testing.T) {
	t.Setenv("XHUB_BENCHMARK_KEY", "sk-unit-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{\"choices\":[{\"message\":{\"content\":\"ok\"}}]}"))
	}))
	defer server.Close()
	for _, tc := range []struct {
		name              string
		cancelled, failed bool
	}{{"success", false, false}, {"cancelled", true, false}, {"threshold", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			dir := t.TempDir()
			args := []string{"-base-url", server.URL, "-model", "m", "-scenarios", "chat", "-concurrency", "1", "-requests", "3", "-warmup", "0", "-out", dir}
			if tc.failed {
				args = append(args, "-max-p95-ms", "0.000001")
			}
			err := run(ctx, args)
			if (err != nil) != (tc.cancelled || tc.failed) {
				t.Fatalf("退出结果错误: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "report.json"))
			if err != nil {
				t.Fatal(err)
			}
			var r report
			if err = json.Unmarshal(data, &r); err != nil {
				t.Fatal(err)
			}
			if r.Passed != (!tc.cancelled && !tc.failed) || r.Cancelled != tc.cancelled {
				t.Fatalf("报告状态错误: %+v", r)
			}
			if _, err = os.Stat(filepath.Join(dir, "report.md")); err != nil {
				t.Fatal(err)
			}
		})
	}
	// 写报告遇到不存在目录应明确报错；不创建任何外部文件。
	if err := writeReport(filepath.Join(t.TempDir(), "missing"), report{}); err == nil {
		t.Fatal("报告写失败未报错")
	}
}
