// 测试库用的 schema 名字。只接受安全的标识符。
package store

import "fmt"

// 给 schema 名加引号。空、过长或带符号的名字直接拒绝。
func quoteIdent(name string) (string, error) {
	if name == "" || len(name) > 63 {
		return "", fmt.Errorf("invalid schema name")
	}
	for _, c := range name {
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return "", fmt.Errorf("invalid schema name")
		}
	}
	return `"` + name + `"`, nil
}
