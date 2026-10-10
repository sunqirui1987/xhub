package iam

import (
	"context"
	"fmt"
	"math"

	"xorm.io/xorm"
)

// RecordTaskUsage 在原请求上保存任务状态、最终正文及首次结算，轮询不增加日志或请求计数。
// 参数 ctx、r、initial、settle：上下文、调用记录、是否创建、是否成功实测结算；返回持久化错误。
// 供网关异步任务调用；事务锁串行化重复查询，原归属固定，完成态不回退，零费用结算也去重。
func (db *DB) RecordTaskUsage(ctx context.Context, r UsageRecord, initial, settle bool) error {
	if r.RequestID == "" || r.Cost < 0 || math.IsNaN(r.Cost) || math.IsInf(r.Cost, 0) || r.PromptTokens < 0 || r.CompletionTokens < 0 {
		return fmt.Errorf("invalid task usage")
	}
	switch r.Status {
	case "executing", "polling", "completed", "failed":
	default:
		return fmt.Errorf("invalid task status %q", r.Status)
	}
	if settle && r.Status != "completed" {
		return fmt.Errorf("task settlement requires completion")
	}
	r.TS = stamp(r.TS)
	return db.tx(ctx, func(s *xorm.Session) error {
		// 请求身份锁覆盖尚未插入的创建，避免并发创建或查询丢失第一次更新。
		if _, err := s.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", r.RequestID); err != nil {
			return err
		}
		var old UsageEvent
		found, err := s.Where("request_id = ?", r.RequestID).Get(&old)
		if err != nil {
			return err
		}
		if !found {
			if !initial {
				return fmt.Errorf("task request log missing")
			}
			// 创建只计一次请求；后续成功查询才提供实测用量和价格快照。
			r.Cost, r.PromptTokens, r.CompletionTokens = 0, 0, 0
			if _, err := insertEvent(s, r); err != nil {
				return err
			}
			if err := putRequestLog(s, r); err != nil {
				return err
			}
			return bumpDaily(s, r)
		}
		if old.KeyID != r.KeyID || old.UserID != r.UserID || old.Model != r.Model {
			return fmt.Errorf("task request ownership mismatch")
		}
		if initial {
			return nil
		}
		if old.TaskSettled || (old.Status == "completed" && !settle) {
			return nil
		}
		if old.Status == "failed" && r.Status != "completed" && r.Status != "failed" {
			return nil
		}
		// 所有归属与开始时间沿用原请求，不能由后续调用重写账单归属。
		r.TS, r.OrganizationID, r.TeamID, r.ProjectID, r.OwnerType = old.TS, old.OrganizationID, old.TeamID, old.ProjectID, old.OwnerType
		if _, err := s.Exec("UPDATE usage_events SET status = ?, http_status = ?, ended_at = ?, duration_ms = ? WHERE request_id = ?", r.Status, r.HTTPStatus, r.EndedAt, r.DurationMS, r.RequestID); err != nil {
			return err
		}
		errorBody := ""
		if r.Status == "failed" {
			errorBody = r.Error
			if errorBody == "" {
				errorBody = r.ResponseBody
			}
		}
		if _, err := s.Exec("UPDATE request_logs SET response_body = ?, error = ?, upstream_response = ? WHERE request_id = ?", r.ResponseBody, errorBody, r.UpstreamResponse, r.RequestID); err != nil {
			return err
		}
		if !settle {
			return nil
		}
		if _, err := s.Exec("UPDATE usage_events SET task_settled = true, cost = ?, prompt_tokens = ?, completion_tokens = ?, price_snapshot = ? WHERE request_id = ?", r.Cost, r.PromptTokens, r.CompletionTokens, r.PriceSnapshot, r.RequestID); err != nil {
			return err
		}
		// 结算累加费用和用量，创建已计入的请求数不再增加。
		if _, err := s.Exec(`UPDATE usage_daily SET cost = cost + ?, prompt_tokens = prompt_tokens + ?, completion_tokens = completion_tokens + ?
   WHERE day = ? AND organization_id = ? AND team_id = ? AND project_id = ? AND user_id = ? AND key_id = ? AND owner_type = ? AND model = ?`,
			r.Cost, r.PromptTokens, r.CompletionTokens, day(old.TS), old.OrganizationID, old.TeamID, old.ProjectID, old.UserID, old.KeyID, ownerType(r), old.Model); err != nil {
			return err
		}
		return addScopeSpend(s, r)
	})
}
