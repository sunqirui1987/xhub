package live

import (
	"context"
	"fmt"
	"time"
)

// RateScope 定义分钟汇总对象；ID 带层级前缀，nil 不设本层上限但仍计数。
type RateScope struct {
	ID, Kind string
	RPM, TPM *int
}

// rateAdmission 是 CheckRatePlan 的 Redis 原子实现，网关通过 AdmitRatePlan 调用。
// KEYS 每节点两个键（RPM、TPM）；ARGV 每节点为 1 起始父下标、RPM、TPM，
// 后接 est、路径长度和叶到根下标；父 0 表示根，限额 -1 表示共享，0 表示禁用。
// reserve/credit 保留与纯函数相同的不变量；两个维度全部通过后才增加整条路径，
// 返回 0 放行或 1 起始“节点×维度”拒绝索引。拒绝不写计数，120 秒 TTL 清理旧桶。
const rateAdmission = `
local count=#KEYS/2
local children={}; local rpm={}; local tpm={}; local rpml={}; local tpml={}; local parents={}
for i=1,count do children[i]={}; rpm[i]=tonumber(redis.call("GET",KEYS[i*2-1]) or "0"); tpm[i]=tonumber(redis.call("GET",KEYS[i*2]) or "0"); local b=(i-1)*3; parents[i]=tonumber(ARGV[b+1]); rpml[i]=tonumber(ARGV[b+2]); tpml[i]=tonumber(ARGV[b+3]) end
for i=1,count do if parents[i]>0 then table.insert(children[parents[i]],i) end end
local est=tonumber(ARGV[count*3+1]); local pathCount=tonumber(ARGV[count*3+2])
for dimension=1,2 do
 local usage=rpm; local limits=rpml; local delta=1
 if dimension==2 then usage=tpm; limits=tpml; delta=est end
 local function reserve(i)
  if limits[i]>=0 then return math.max(0,limits[i]-usage[i]) end
  local sum=0; for _,c in ipairs(children[i]) do sum=sum+reserve(c) end; return sum
 end
 local child=0; local credit=0
 for p=1,pathCount do
  local i=tonumber(ARGV[count*3+2+p])
  if limits[i]>=0 then
   local reserved=0; for _,c in ipairs(children[i]) do local amount=reserve(c); if c==child then amount=math.max(0,amount-credit) end; reserved=reserved+amount end
   if limits[i]==0 or usage[i]+reserved+delta>limits[i] then return (i-1)*2+dimension end
   credit=math.max(0,limits[i]-usage[i])
  end
  child=i
 end
end
for p=1,pathCount do
 local i=tonumber(ARGV[count*3+2+p]); redis.call("INCR",KEYS[i*2-1]); redis.call("PEXPIRE",KEYS[i*2-1],120000); redis.call("INCRBY",KEYS[i*2],est); redis.call("PEXPIRE",KEYS[i*2],120000)
end
return 0
`

// AdmitRates 原子申请四层 RPM/TPM；参数为范围、估算与时钟，返回拒绝字段或存储错误。
// 调用：网关推理；固定 UTC 分钟桶，多实例共享同一 Redis，无上限层也记录放行用量。
func (c *Client) AdmitRates(scopes []RateScope, est int, now time.Time) (string, error) {
	plan := RatePlan{}
	for i, scope := range scopes {
		plan.Nodes = append(plan.Nodes, RateNode{RateScope: scope, Parent: -1})
		plan.Path = append(plan.Path, i)
	}
	return c.AdmitRatePlan(plan, est, now)
}

// AdmitRatePlan 原子校验并增加当前归属树的调用路径；plan 由 IAM 验证，est 为入站估算，now 确定 UTC 分钟。
// 返回拒绝的层级/字段或 Redis 错误；推理准入调用，空路径无操作，负估算按零处理。
// 同一 Redis 的多实例共享窗口；脚本失败由网关返回 503，禁止无计数降级放行。
// 副作用：只为成功路径增加 RPM/TPM 并设置 120 秒 TTL，不修改兄弟计数或额度配置。
func (c *Client) AdmitRatePlan(plan RatePlan, est int, now time.Time) (string, error) {
	if len(plan.Path) == 0 {
		return "", nil
	}
	est = max(0, est)
	keys := []string{}
	args := []any{}
	for _, node := range plan.Nodes {
		prefix := fmt.Sprintf("xhub:rate:%s:%d:", node.ID, now.Unix()/60)
		keys = append(keys, prefix+"rpm", prefix+"tpm")
		rpm, tpm := -1, -1
		if node.RPM != nil {
			rpm = *node.RPM
		}
		if node.TPM != nil {
			tpm = *node.TPM
		}
		args = append(args, node.Parent+1, rpm, tpm)
	}
	args = append(args, est, len(plan.Path))
	for _, i := range plan.Path {
		args = append(args, i+1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	index, err := c.rdb.Eval(ctx, rateAdmission, keys, args...).Int()
	if err != nil {
		return "", err
	}
	if index == 0 {
		return "", nil
	}
	field := "rpm_limit"
	if index%2 == 0 {
		field = "tpm_limit"
	}
	return plan.Nodes[(index-1)/2].Kind + " " + field, nil
}
