package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/goplus/ixgo"
	_ "github.com/goplus/ixgo/pkg/fmt"
	"github.com/goplus/ixgo/xgobuild"
	lru "github.com/hashicorp/golang-lru/v2"
	"go/ast"
	"go/parser"
	"go/types"
	"golang.org/x/tools/go/ssa"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
)

// customAPI 是唯一允许脚本导入的虚拟包路径，注册函数在这里提供受控能力。
const customAPI = "xhub/guardrail"

// maxCustomText 限制文本、JSON 及 HTTP 正文为 1 MiB，避免输入和结果无限膨胀。
const maxCustomText = 1 << 20

// 自定义脚本只供可信管理员使用。导入白名单与执行时限限制能力和运行时间，
// 但解释器仍在网关进程内，不能提供独立内存配额或进程隔离。
// 编译和解释器构建串行化；缓存只共享不可变程序，不共享脚本全局变量。
var customCompileMu sync.Mutex
var customCache, _ = lru.New[string, *customProgram](64)

// customProgram 保存可重用的编译产物；network 决定是否需要 10 秒网络总预算。
type customProgram struct {
	ctx     *ixgo.Context
	pkg     *ssa.Package
	network bool
}

// init 注册 xhub/guardrail 的函数签名及不依赖请求上下文的实现。网络函数仅用于声明类型，实际调用在编译后绑定到本次解释器。
// 参数：无。
// 返回：无；注册后仅允许脚本使用受控能力。
// 调用：Go 包初始化；compileCustom 使用注册结果。
// 测试：custom_test.go、primitives_test.go。
func init() {
	funcs := map[string]reflect.Value{}
	for name, fn := range map[string]any{
		"Allow": func() map[string]any { return map[string]any{"action": "allow"} },
		"Block": func(reason string) map[string]any { return map[string]any{"action": "block", "reason": reason} },
		"Flag": func(reason string, metadata map[string]any) map[string]any {
			return map[string]any{"action": "flag", "reason": reason, "metadata": metadata}
		},
		"JSONParse": jsonParse, "JSONStringify": jsonStringify,
		// 网络函数不能在进程级注册表中捕获请求上下文；此处仅声明签名。
		"HTTPGet":     func(url string, headers map[string]string, timeout int) map[string]any { return nil },
		"HTTPPost":    func(url string, body any, headers map[string]string, timeout int) map[string]any { return nil },
		"HTTPRequest": func(url, method string, headers map[string]string, body any, timeout int) map[string]any { return nil },
		"LLMChat":     func(base, key, model string, messages []map[string]any, timeout int) map[string]any { return nil },
		"Modify":      func(texts []string) map[string]any { return map[string]any{"action": "modify", "texts": texts} },
		"Contains":    strings.Contains, "Lower": strings.ToLower, "Trim": strings.TrimSpace,
		"RegexMatch": func(text, pattern string) bool { return customRegex(text, pattern).MatchString(text) },
		"RegexReplace": func(text, pattern, replacement string) string {
			re := customRegex(text, pattern)
			if re.MatchString("") {
				panic("pattern must not match empty text")
			}
			if len(replacement) > 4096 || len(text)+len(re.FindAllStringIndex(text, -1))*len(replacement) > maxCustomText {
				panic("modified text too large")
			}
			return re.ReplaceAllStringFunc(text, func(string) string { return replacement })
		},
	} {
		funcs[name] = reflect.ValueOf(fn)
	}
	ixgo.RegisterPackage(&ixgo.Package{Name: "guardrail", Path: customAPI, Funcs: funcs})
}

// customRegex 编译 Go RE2 表达式，统一限制文本与表达式长度。
// 参数：text：被检查的文本，最多 1 MiB；pattern：RE2 表达式，最多 4096 字节。
// 返回：编译后的表达式；格式或长度非法时 panic，由脚本执行器转换为错误。
// 调用：RegexMatch、RegexReplace primitives。
// 测试：custom_test.go。
func customRegex(text, pattern string) *regexp.Regexp {
	if len(text) > maxCustomText || len(pattern) > 4096 {
		panic("regex input too large")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(err)
	}
	return re
}

// policyImporter 为 XGo 转换和 SSA 类型检查提供包白名单。
type policyImporter struct{ base types.Importer }

// Import 限制类型导入器可解析的包；fmt 仅用于 XGo 转换阶段，用户 AST 仍禁止导入 fmt。
// 参数：path：待解析的包路径；接收者 base：iXGo 原始类型导入器。
// 返回：类型包及错误；未授权的包直接拒绝。
// 调用：compileCustom 的 XGo 转换及 SSA 构建。
// 测试：custom_test.go 的导入限制用例。
func (p policyImporter) Import(path string) (*types.Package, error) {
	if path != customAPI && path != "fmt" {
		return nil, fmt.Errorf("import %q is not allowed; use xhub/guardrail helpers", path)
	}
	return p.base.Import(path)
}

// compileCustom 将 XGo 源码转换为 Go AST，再校验受限语法、绑定网络函数并构建可缓存的 SSA 程序。
// 参数：code：1–32768 字节的源码，可省略 package main；必须声明约定的 ApplyGuardrail。
// 返回：program：只读编译结果；err：解析、类型、语法限制或入口签名错误。失败不写入缓存。
// 调用：Validate、testCustomCode、runCustomDetailed。
// 测试：custom_test.go、primitives_test.go。
func compileCustom(code string) (program *customProgram, err error) {
	defer func() {
		if v := recover(); v != nil {
			program = nil
			err = fmt.Errorf("script compilation failed: %v", v)
		}
	}()
	if strings.Contains(code, "__xhub") {
		return nil, fmt.Errorf("__xhub is reserved for the runtime")
	}
	if len(code) == 0 || len(code) > 32768 {
		return nil, fmt.Errorf("custom_code must contain 1 to 32768 bytes")
	}
	customCompileMu.Lock()
	defer customCompileMu.Unlock()
	if cached, ok := customCache.Get(code); ok {
		return cached, nil
	}
	ctx := ixgo.NewContext(ixgo.SupportMultipleInterp | ixgo.DisableCustomBuiltin | ixgo.DisableAutoLoadPatchs)
	ctx.Importer = policyImporter{ctx.Importer}
	source := code
	if !strings.HasPrefix(strings.TrimSpace(source), "package ") {
		source = "package main\n" + source
	}
	xc := xgobuild.NewContext(ctx)
	xc.Importer = ctx.Importer
	xp, err := xc.ParseFile("guardrail.xgo", source)
	if err != nil {
		return nil, err
	}
	generated, err := xp.ToSource()
	if err != nil {
		return nil, err
	}
	file, err := parser.ParseFile(ctx.FileSet, "guardrail.go", generated, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.ImportSpec:
			if v.Path.Value != "\"xhub/guardrail\"" {
				err = fmt.Errorf("only xhub/guardrail may be imported")
			}
		case *ast.GoStmt, *ast.DeferStmt, *ast.ChanType, *ast.SendStmt, *ast.SelectStmt:
			err = fmt.Errorf("goroutines, defer and channels are not supported")
		case *ast.FuncDecl:
			if v.Name.Name == "init" || (v.Name.Name == "main" && (v.Body == nil || len(v.Body.List) > 0)) {
				err = fmt.Errorf("define only ApplyGuardrail and helper functions, no init/main")
			}
		case *ast.Comment:
			if strings.HasPrefix(v.Text, "//go:") {
				err = fmt.Errorf("compiler directives are not allowed")
			}
		}
		return err == nil
	})
	if err != nil {
		return nil, err
	}
	// 把网络函数引用改为解释器私有变量，支持点导入及包别名导入。
	// 原源码禁止 __xhub，防止用户访问或覆盖注入变量。
	network := false
	alias := ""
	for _, spec := range file.Imports {
		if spec.Name != nil {
			alias = spec.Name.Name
		} else {
			alias = "guardrail"
		}
	}
	names := map[string]bool{"HTTPRequest": true, "HTTPGet": true, "HTTPPost": true, "LLMChat": true}
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == alias && names[sel.Sel.Name] {
				network = true
				sel.X = ast.NewIdent("__xhubBindings")
			}
			return true
		}
		if id, ok := n.(*ast.Ident); ok && alias == "." && names[id.Name] {
			network = true
			id.Name = "__xhub" + id.Name
		}
		return true
	})
	// 注入与公开函数签名一致的变量；RunInit 后绑定当前请求的闭包。
	bindings, e := parser.ParseFile(ctx.FileSet, "bindings.go", `package main
var __xhubHTTPRequest func(string,string,map[string]string,any,int) map[string]any
var __xhubHTTPGet func(string,map[string]string,int) map[string]any
var __xhubHTTPPost func(string,any,map[string]string,int) map[string]any
var __xhubLLMChat func(string,string,string,[]map[string]any,int) map[string]any
var __xhubBindings struct {
 HTTPRequest func(string,string,map[string]string,any,int) map[string]any
 HTTPGet func(string,map[string]string,int) map[string]any
 HTTPPost func(string,any,map[string]string,int) map[string]any
 LLMChat func(string,string,string,[]map[string]any,int) map[string]any
}`, 0)
	if e != nil {
		return nil, e
	}
	file.Decls = append(file.Decls, bindings.Decls...)
	pkg, err := ctx.LoadAstFile("main", file)
	if err != nil {
		return nil, err
	}
	fn := pkg.Func("ApplyGuardrail")
	if fn == nil || fn.Signature.Params().Len() != 3 || fn.Signature.Results().Len() != 1 {
		return nil, fmt.Errorf("define func ApplyGuardrail(texts []string, requestData map[string]any, inputType string) map[string]any")
	}
	want := []string{"[]string", "map[string]any", "string"}
	for i, t := range want {
		if strings.ReplaceAll(fn.Signature.Params().At(i).Type().String(), "interface{}", "any") != t {
			return nil, fmt.Errorf("ApplyGuardrail argument %d must be %s", i+1, t)
		}
	}
	if strings.ReplaceAll(fn.Signature.Results().At(0).Type().String(), "interface{}", "any") != "map[string]any" {
		return nil, fmt.Errorf("ApplyGuardrail must return map[string]any")
	}
	program = &customProgram{ctx: ctx, pkg: pkg, network: network}
	customCache.Add(code, program)
	return program, nil
}

// runCustom 提供不需要 Flag 附加元数据的兼容执行入口。
// 参数：code：XGo 源码；texts：有序文本；body：请求模型和 metadata；inputType：当前仅 request。
// 返回：action、reason、out：执行结果；err：编译或运行错误。Flag 元数据由此包装层丢弃。
// 调用：现有测试及兼容调用；实际请求使用 runCustomDetailed。
// 测试：custom_test.go。
func runCustom(code string, texts []string, body map[string]any, inputType string) (action, reason string, out []string, err error) {
	action, reason, out, _, err = runCustomDetailed(code, texts, body, inputType)
	return
}

// runCustomDetailed 为每次请求创建独立 iXGo 实例，复制输入、注入网络上下文并校验脚本结果。
// 参数：code：源码；texts：最多 1 MiB/10000 段；body：model 和 metadata 来源；inputType：request。
// 返回：action：allow/block/modify/flag；reason：说明；out：位置对应的文本；metadata：Flag 的 JSON 副本；err：失败原因。
// 调用：runRuleDetailed、testCustomCode、runCustom。
// 测试：custom_test.go、primitives_test.go、engine_test.go。
func runCustomDetailed(code string, texts []string, body map[string]any, inputType string) (action, reason string, out []string, metadata map[string]any, err error) {
	program, err := compileCustom(code)
	if err != nil {
		return "", "", nil, nil, err
	}
	if inputType != "request" {
		return "", "", nil, nil, fmt.Errorf("only request/pre_call is supported")
	}
	size := 0
	for _, text := range texts {
		size += len(text)
	}
	if size > maxCustomText || len(texts) > 10000 {
		return "", "", nil, nil, fmt.Errorf("custom guardrail input too large")
	}
	customCompileMu.Lock()
	interp, err := program.ctx.NewInterp(program.pkg)
	customCompileMu.Unlock()
	if err != nil {
		return "", "", nil, nil, err
	}
	defer interp.UnsafeRelease()
	type answer struct {
		value any
		err   error
	}
	done := make(chan answer, 1)
	// 只暴露模型和客户端 metadata；深复制防止脚本修改真实请求元数据。
	request := map[string]any{"model": body["model"]}
	if metadata := body["metadata"]; metadata != nil {
		raw, e := json.Marshal(metadata)
		if e != nil {
			return "", "", nil, nil, fmt.Errorf("invalid metadata: %w", e)
		}
		if len(raw) > maxCustomText {
			return "", "", nil, nil, fmt.Errorf("metadata too large")
		}
		var copy any
		if e := json.Unmarshal(raw, &copy); e != nil {
			return "", "", nil, nil, e
		}
		request["metadata"] = copy
	}
	// 网络脚本共享一个总预算，多次 HTTP 调用不能各自延长总时限。
	deadline := 100 * time.Millisecond
	if program.network {
		deadline = 10 * time.Second
	}
	invocation, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	copied := append([]string{}, texts...)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				done <- answer{err: fmt.Errorf("script execution failed: %v", v)}
			}
		}()
		if e := interp.RunInit(); e != nil {
			done <- answer{err: e}
			return
		}
		bindNetwork(interp, invocation)
		v, e := interp.RunFunc("ApplyGuardrail", copied, request, inputType)
		done <- answer{v, e}
	}()
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	var result answer
	select {
	case result = <-done:
	case <-timer.C:
		// 先取消正在等待的 HTTP，再中止解释器；等待协程退出后才能释放实例。
		cancel()
		interp.Abort()
		<-done
		return "", "", nil, nil, fmt.Errorf("script exceeded %s execution deadline", deadline)
	}
	if result.err != nil {
		return "", "", nil, nil, result.err
	}
	data, ok := result.value.(map[string]any)
	if !ok {
		return "", "", nil, nil, fmt.Errorf("return Allow(), Block(reason), Flag(reason, metadata), or Modify(texts)")
	}
	action, _ = data["action"].(string)
	reason, _ = data["reason"].(string)
	out = texts
	switch action {
	case "allow", "block":
	case "flag":
		metadata, _ = data["metadata"].(map[string]any)
		raw, e := json.Marshal(metadata)
		if e != nil || len(raw) > maxCustomText {
			return "", "", nil, nil, fmt.Errorf("invalid flag metadata")
		}
		if e = json.Unmarshal(raw, &metadata); e != nil {
			return "", "", nil, nil, e
		}
	case "modify":
		out, ok = data["texts"].([]string)
		if !ok || len(out) != len(texts) {
			return "", "", nil, nil, fmt.Errorf("Modify must return one string per input text")
		}
		size = 0
		for _, text := range out {
			size += len(text)
		}
		if size > maxCustomText {
			return "", "", nil, nil, fmt.Errorf("modified text too large")
		}
	default:
		return "", "", nil, nil, fmt.Errorf("invalid guardrail action")
	}
	return action, reason, out, metadata, nil
}
