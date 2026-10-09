package models

import (
	"encoding/json"
	"fmt"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/router"
	"io"
	"net/http"
	"sort"
)

// loadRoutingGroups 读取独立命名组配置；参数为宿主，返回有序列表或存储/契约错误，无写入副作用。
func loadRoutingGroups(s Host) ([]router.Group, error) {
	raw, err := s.RecordStore().ListConfig("routing_groups")
	if err != nil {
		return nil, err
	}
	out := []router.Group{}
	for name, value := range raw {
		g, err := router.ParseGroup(value)
		if err != nil {
			return nil, err
		}
		if g.Name != name {
			return nil, fmt.Errorf("invalid group identity")
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// RoutingGroups 提供平台管理员的路由组列表与单组创建、更新、删除；参数为宿主和真实 HTTP 请求。
// 名称在创建后固定，更新删除要求已存在；校验与持久化在模型锁内完成，失败不会部分修改。
func RoutingGroups(s Host, w http.ResponseWriter, r *http.Request) {
	p := s.RequireManage(w, r)
	if p == nil {
		return
	}
	if !p.PlatformAdmin() {
		httpx.WriteError(w, 403, "forbidden", "platform administrator required")
		return
	}
	var group router.Group
	if r.Method != "GET" {
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		dec.DisallowUnknownFields()
		if r.Method == "DELETE" {
			var input struct {
				Name string `json:"group_name"`
			}
			if err := dec.Decode(&input); err != nil || input.Name == "" {
				httpx.WriteError(w, 400, "invalid_request", "group_name required")
				return
			}
			group.Name = input.Name
		} else if err := dec.Decode(&group); err != nil {
			httpx.WriteError(w, 400, "invalid_request", err.Error())
			return
		}
		var trailing any
		if dec.Decode(&trailing) != io.EOF {
			httpx.WriteError(w, 400, "invalid_request", "exactly one JSON object required")
			return
		}
	}
	s.LockModels()
	defer s.UnlockModels()
	groups, err := loadRoutingGroups(s)
	if err != nil {
		httpx.WriteError(w, 503, "unavailable", "routing groups unavailable")
		return
	}
	if r.Method == "GET" {
		httpx.WriteJSON(w, 200, map[string]any{"data": groups})
		return
	}
	existing := -1
	for i, g := range groups {
		if g.Name == group.Name {
			existing = i
			break
		}
	}
	if r.Method == "POST" && existing >= 0 {
		httpx.WriteError(w, 409, "conflict", "routing group already exists")
		return
	}
	if r.Method != "POST" && existing < 0 {
		httpx.WriteError(w, 404, "not_found", "routing group not found")
		return
	}
	if r.Method == "DELETE" {
		if s.RecordStore() == nil {
			httpx.WriteError(w, 503, "unavailable", "routing group store unavailable")
			return
		}
		if err = s.RecordStore().DeleteConfig("routing_groups", group.Name); err != nil {
			httpx.WriteError(w, 503, "unavailable", err.Error())
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"deleted": group.Name})
		return
	}
	next := append([]router.Group(nil), groups...)
	if existing >= 0 {
		next[existing] = group
	} else {
		next = append(next, group)
	}
	list := []config.ModelEntry{}
	for _, dep := range *s.ModelTable() {
		if !nonModelEntry(dep) {
			list = append(list, dep)
		}
	}
	if err = router.ValidateGroups(next, list); err != nil {
		httpx.WriteError(w, 400, "invalid_request", err.Error())
		return
	}
	if s.RecordStore() == nil {
		httpx.WriteError(w, 503, "unavailable", "routing group store unavailable")
		return
	}
	if err = s.RecordStore().PutConfig("routing_groups", group.Name, group); err != nil {
		httpx.WriteError(w, 503, "unavailable", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, group)
}

// checkRoutingGroupRemoval 拒绝删除或改名仍被组引用的最后部署；调用方持锁，返回引用冲突或存储错误。
func checkRoutingGroupRemoval(s Host, name string) error {
	groups, err := loadRoutingGroups(s)
	if err != nil {
		return err
	}
	for _, group := range groups {
		for _, member := range group.Models {
			if member == name {
				return fmt.Errorf("remove model %q from routing group %q before deleting or renaming its last deployment", name, group.Name)
			}
		}
	}
	return nil
}

// checkRoutingGroupName 保证新增或改名模型不会遮蔽现有组名；调用方持锁，无写入副作用。
func checkRoutingGroupName(s Host, name string) error {
	groups, err := loadRoutingGroups(s)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.Name == name {
			return fmt.Errorf("model name %q shadows a routing group", name)
		}
	}
	return nil
}

// checkRoutingAllocationRemoval 防止删除或改名使组权重引用失效；调用方持锁，错误要求先编辑组。
func checkRoutingAllocationRemoval(s Host, id string) error {
	groups, err := loadRoutingGroups(s)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.Args != nil {
			for _, allocation := range group.Args.Allocations {
				if allocation.DeploymentID == id {
					return fmt.Errorf("remove deployment %q allocation from routing group %q first", id, group.Name)
				}
			}
		}
	}
	return nil
}
