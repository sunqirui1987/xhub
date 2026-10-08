// Package templateauth shares the visibility check across template binding and
// ordinary organization, team and key saves.
package templateauth

import (
	"net/http"
	"strings"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/iam"
	"github.com/sunqirui1987/xhub/internal/logx"
)

type Host interface {
	// Identity 返回模板存储。
	// 参数：无。返回：身份库。
	// 调用：Selection。测试：template_selection_regression_test.go。
	Identity() *iam.DB
	// Authorize 校验当前调用方可见的模板范围。
	// 参数：请求、身份、动作和目标对象。返回：授权错误。
	// 调用：Selection。测试：template_selection_regression_test.go。
	Authorize(*http.Request, *auth.Principal, authz.Action, authz.Object) error
}

// Selection checks the selected template before any scope is mutated. Empty
// means inherit and needs no template read. Ownership must be loaded explicitly:
// route templates are not resolved by Authorize from their ID alone.
// 参数 h、r、p、id：身份存储、请求、调用方及选中的模板 id。
// 返回：模板不可见、不存在或读取失败时的错误。
// 调用：密钥、团队、组织保存及模板绑定。测试：template_selection_regression_test.go。
func Selection(h Host, r *http.Request, p *auth.Principal, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	row, err := h.Identity().GetRouteTemplate(r.Context(), id)
	if err != nil {
		logx.Error("route template selection lookup failed err=%v", err)
		return &authz.InternalError{Err: err}
	}
	if row == nil {
		return authz.ErrNotFound
	}
	obj := authz.Object{Type: authz.ObjectRouteTemplate, ID: row.ID}
	if row.OrganizationID != nil {
		obj.OrgID = *row.OrganizationID
	}
	if row.TeamID != nil {
		obj.TeamID = *row.TeamID
	}
	return h.Authorize(r, p, authz.ActionRouteTemplateRead, obj)
}
