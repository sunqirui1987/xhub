package router

import (
	"math"
	"sync"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// SplitState remembers how traffic has been divided between deployments that
// share one public model name.
//
// This is the router's only mutable state. Every other strategy picks by looking
// at the pool, so the same input gives the same answer; an even split has to
// remember what it chose last time, which is exactly why it needs care: two
// deployments at 50/50 must alternate rather than returning the first one
// forever, which is what the weight strategy does today.
//
// The cursor is per deployment id, not per alias. Two aliases that share one
// deployment then count against that deployment once, which is what an operator
// setting a percentage means: "a third of the traffic to this endpoint".
type SplitState struct {
	mu sync.Mutex
	// current is the running score per deployment id - the nginx smooth weighted
	// round-robin accumulator.
	current map[string]float64
}

// NewSplitState returns an empty split state. One instance is shared by a
// process. The counters are per-process, so several replicas each converge on
// the configured ratio independently rather than coordinating one global
// schedule; the aggregate ratio is right either way.
// 参数：无。
// 返回 *SplitState（*SplitState）：可以直接放进 router.State 的分流状态。
// 调用：SharedSplit 和 gateway/split 的测试。
// 测试：split_test.go
func NewSplitState() *SplitState {
	return &SplitState{current: map[string]float64{}}
}

var (
	sharedOnce  sync.Once
	sharedSplit *SplitState
)

// SharedSplit returns the process-wide split cursors.
//
// The router State is rebuilt for every request, so the cursors cannot live on
// it - a split that forgot where it was would send every request to the same
// deployment. One shared instance is what makes the ratio hold across requests.
// 参数：无。
// 返回 *SplitState（*SplitState）：整个进程共用的分流状态。
// 调用：dataplane/live.go 组装路由状态时。
// 测试：split_test.go
func SharedSplit() *SplitState {
	sharedOnce.Do(func() { sharedSplit = NewSplitState() })
	return sharedSplit
}

// PickWeighted chooses the deployment that is furthest behind its share.
//
// This is smooth weighted round-robin: each turn every candidate's score grows
// by its weight, the highest score wins, and the winner's score drops by the
// total. Over any ten draws a 3:7 split lands exactly 3 and 7 rather than merely
// averaging that over a long run, which matters because a caller that watches
// ten consecutive requests should see the ratio they configured.
//
// A candidate that is not available has its score forgotten, so it re-enters at
// zero rather than immediately claiming the share it accrued while it was
// cooling down. That would otherwise send the first requests after a recovery
// all to the deployment that just came back.
//
// 参数 ids（[]string）：候选部署的 id，和 weights 一一对应；weights（[]float64）：每条候选的权重，非正数表示不接流量；
// available（[]bool）：每条候选此刻是否可以接流量。
//
// 返回 int（int）：选中的下标。没有可接流量的候选时是 -1。
// 调用：Pick 的 split 分支。
// 测试：split_test.go
func (s *SplitState) PickWeighted(ids []string, weights []float64, available []bool) int {
	if s == nil {
		// Reaching here means the strategy was selected but no split state was
		// installed, so traffic will fall back to a single deployment and the
		// configured ratio silently stops applying.
		logx.Error("weighted split has no state; traffic will not follow the configured weights")
		return -1
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	total := 0.0
	for i, id := range ids {
		if !available[i] || weights[i] <= 0 || math.IsNaN(weights[i]) || math.IsInf(weights[i], 0) {
			delete(s.current, id)
			continue
		}
		total += weights[i]
	}
	if total <= 0 {
		return -1
	}

	best := -1
	bestScore := math.Inf(-1)
	for i, id := range ids {
		if !available[i] || weights[i] <= 0 || math.IsNaN(weights[i]) || math.IsInf(weights[i], 0) {
			continue
		}
		score := s.current[id] + weights[i]
		s.current[id] = score
		if score > bestScore {
			bestScore = score
			best = i
		}
	}
	if best < 0 {
		return -1
	}
	s.current[ids[best]] -= total
	return best
}

// Forget drops a deployment's cursor. A deployment that is removed or renamed
// would otherwise leave its score behind forever, which is a slow leak in a
// process that runs for weeks.
// 参数 id（string）：要忘掉的部署 id。
// 返回：无。
// 调用：目前没有调用点；保留给部署被删除时的清理。
// 测试：split_test.go
func (s *SplitState) Forget(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.current, id)
}
