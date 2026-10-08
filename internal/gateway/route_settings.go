package gateway

import (
	"context"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/logx"
)

// RouteSettingsFor resolves the router settings one request runs under.
//
// The narrow scope is already known: keyBudgetOK walked key, team and
// organization for the budget chain and recorded both the first scope that names
// a template and which scope that was. So this does not walk anything - it reads
// the one template that walk already chose, or the platform document when none
// was chosen.
//
// There is deliberately no process-wide cache. A cache would be fastest to build
// around the template row, and it would also be the thing that hands a stale
// template to the requests immediately after an operator edits one - the exact
// moment they are looking at the console to check that the edit took. One
// primary-key read per inference request is the cheaper mistake.
//
// 参数 p（*auth.Principal）：已经解析的调用方，其 RouteTemplateID 由预算链填入。
// 返回 prefs.RouteSettings（prefs.RouteSettings）：这次请求生效的设置和来源。
// 调用：gateway 的请求路径。
// 测试：route_settings_test.go
func (s *Server) RouteSettingsFor(p *auth.Principal) prefs.RouteSettings {
	platform := s.RouterDocument()
	if p == nil || s.IAM == nil || p.RouteTemplateID == "" {
		return prefs.PlatformSettings(platform)
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
		logx.Error("route template is gone id=%s; falling back to the platform default", p.RouteTemplateID)
		return prefs.PlatformSettings(platform)
	}
	source := p.RouteTemplateSource
	if source == "" {
		source = prefs.PlatformSource
	}
	return prefs.RouteSettings{
		Settings:     row.Settings(),
		TemplateID:   row.ID,
		TemplateName: row.Name,
		Source:       source,
	}
}
