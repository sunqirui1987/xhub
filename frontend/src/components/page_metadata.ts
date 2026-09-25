/**
 * Page metadata for UI Settings configuration
 * This file contains descriptions and metadata for all navigation pages
 */

// Page descriptions for UI Settings configuration
export const pageDescriptions: Record<string, string> = {
  "api-keys": "管理用于 API 访问与鉴权的虚拟密钥",
  "llm-playground": "交互式调试台，用于测试 LLM 请求",
  models: "配置并管理 LLM 模型与端点",
  agents: "创建并管理智能体",
  agentic: "管理智能体资源：智能体、工作流运行与记忆",
  workflows: "跟踪并查看持久化工作流运行历史",
  "mcp-servers": "配置 Model Context Protocol 服务器",
  memory: "查看并管理 /v1/memory 下的智能体记忆",
  guardrails: "配置内容审核与安全护栏",
  policies: "定义访问控制与用量策略",
  "search-tools": "配置 RAG 搜索与检索工具",
  "tool-policies": "配置工具使用策略与权限",
  "vector-stores": "管理用于嵌入的向量库",
  new_usage: "查看用量分析与指标",
  "cost-optimization": "跟踪并配置省钱能力：提示压缩、缓存与自动路由",
  logs: "查看请求与响应日志",
  "guardrails-monitor": "监控护栏表现并查看日志",
  users: "管理内部用户账号与权限",
  teams: "创建并管理用于访问控制的团队",
  organizations: "管理组织及其成员",
  projects: "管理团队内的项目",
  "access-groups": "管理基于角色的访问组",
  api_ref: "浏览 API 文档与端点",
  "model-hub-table": "浏览可用的 AI 模型与供应商",
  "learning-resources": "查看教程与文档",
  caching: "配置响应缓存与协调 Redis",
  "transform-request": "配置请求转换规则",
  "cost-tracking": "跟踪并分析 API 成本",
  "tag-management": "用标签组织资源",
  prompts: "管理并版本化提示词模板",
  skills: "浏览并管理 Claude Code 技能",
  usage: "查看旧版用量控制台",
  "router-settings": "配置路由与负载均衡",
  "logging-and-alerts": "配置日志与告警",
  "admin-panel": "打开管理员面板与设置",
};

export interface PageMetadata {
  page: string;
  label: string;
  group: string;
  description: string;
}
