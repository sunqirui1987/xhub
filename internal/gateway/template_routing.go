package gateway

import (
	"encoding/json"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/router"
)

// ValidateRoutingTemplate 在身份模板保存前锁定目录并校验引用；参数为完整JSON，返回错误，无写入。
// 由模板创建和更新调用；目录随后变化时数据面仍会重新校验以防悬空引用执行。
func (s *Server) ValidateRoutingTemplate(encoded string) error {
	var doc map[string]any
	if err := json.Unmarshal([]byte(encoded), &doc); err != nil {
		return err
	}
	s.LockModels()
	models := append([]config.ModelEntry(nil), (*s.ModelTable())...)
	s.UnlockModels()
	return router.Compile(router.RouteSettings{Settings: doc}, s.RecordStore(), models).Err
}
