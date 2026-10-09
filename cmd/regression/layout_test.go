package regression

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestProductionModulesExcludeRegressionFixtures 验证生产模块与回归辅助代码的目录及依赖边界。
// 参数：t 为当前测试；返回：无；调用：后台回归套件。
// 前置：可读取仓库源码；结果：旧目录不存在且生产代码不导入回归包；只读检查无需清理。
func TestProductionModulesExcludeRegressionFixtures(t *testing.T) {
	root := filepath.Dir(moduleInternal(t))
	for _, name := range []string{"providerconfig", "testsupport", "regression"} {
		path := filepath.Join(root, "internal", name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("internal 不应保留回归辅助目录 %s：%v", name, err)
		}
	}
	for _, name := range []string{"seed", "pricedata"} {
		path := filepath.Join(root, "cmd", name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("cmd 不应保留已移除的开发命令 %s：%v", name, err)
		}
	}
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// 回归目录自身可以使用夹具；其余生产入口不能依赖测试配置或数据库清理工具。
			if entry.IsDir() {
				if path == filepath.Join(root, "cmd", "regression") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range file.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return err
				}
				const regressionPath = "github.com/sunqirui1987/xhub/cmd/regression"
				if importPath == regressionPath || strings.HasPrefix(importPath, regressionPath+"/") {
					t.Errorf("生产代码 %s 不应依赖回归包 %s", path, importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("检查 %s 的生产依赖边界失败：%v", dir, err)
		}
	}
}

// moduleInternal 向上寻找模块根目录，供目录边界测试定位 internal。
// 参数：t 为当前测试；返回：internal 的绝对路径；调用：生产模块目录与依赖边界测试。
// 工作目录不可读或找不到 go.mod 时终止测试；只读检查无需清理。
func moduleInternal(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "internal")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
