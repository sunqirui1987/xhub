package gateway

// contains 判断列表是否含目标字符串；参数 list、want 为名单与目标，返回匹配结果。
// 调用：网关可见性测试；无副作用，空名单返回 false。
func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
