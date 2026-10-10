package gateway

import (
	"context"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/router"
)

// RouteSettingsFor 读取身份归属链已经选定的客户模板，并合并公开模型的默认权重与回退配置。
// 不缓存模板和模型策略，确保控制台保存后下一次请求立即使用最新配置；无模板时使用内置设置。
// 参数 p（*auth.Principal）：已经解析的调用方；密钥预算检查或会话只读归属解析填入 RouteTemplateID。
// 返回 prefs.RouteSettings：本次生效的设置和来源；读取或解析失败通过 Err 返回，数据面应拒绝执行。
// 调用：gateway 的请求路径。
// 测试：route_settings_test.go
func (s *Server) RouteSettingsFor(p *auth.Principal) (result prefs.RouteSettings) {
	// 网关只负责身份模板读取与目录快照；所有策略合并在router中共用。
	defer func() {
		s.LockModels()
		models := append([]config.ModelEntry(nil), (*s.ModelTable())...)
		s.UnlockModels()
		result = router.Compile(result, s.RecordStore(), models)
	}()
	if p == nil || s.IAM == nil || p.RouteTemplateID == "" {
		return prefs.BuiltinSettings()
	}
	row, err := s.IAM.GetRouteTemplate(context.Background(), p.RouteTemplateID)
	if err != nil {
		logx.Error("route template lookup failed id=%s err=%v", p.RouteTemplateID, err)
		return prefs.RouteSettings{Err: err, TemplateID: p.RouteTemplateID}
	}
	if row == nil {
		// The template was deleted after the chain recorded it. The requests
		// that follow land on the platform default, which is the same state as a
		// scope that selects nothing - the state the operator left behind when
		// they deleted it.
		logx.Error("route template is gone id=%s; falling back to model defaults", p.RouteTemplateID)
		return prefs.BuiltinSettings()
	}
	source := p.RouteTemplateSource
	if source == "" {
		source = prefs.BuiltinSource
	}
	return prefs.RouteSettings{
		Settings:     row.Settings(),
		TemplateID:   row.ID,
		TemplateName: row.Name,
		Source:       source,
	}
}
