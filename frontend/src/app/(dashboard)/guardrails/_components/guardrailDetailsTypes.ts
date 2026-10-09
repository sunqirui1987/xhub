/**
 * 用途：描述护栏设置接口中详情页使用的实体、动作、执行阶段和历史内容过滤配置。
 * 调用：GuardrailInfoView 加载设置并向旧版配置组件传递支持范围。
 * 约束：这里只声明类型，不为缺失的服务能力补默认实现。
 */
export interface GuardrailUISettings {
  supported_entities: string[];
  supported_actions: string[];
  pii_entity_categories: Array<{
    category: string;
    entities: string[];
  }>;
  supported_modes: string[];
  content_filter_settings?: {
    prebuilt_patterns: Array<{
      name: string;
      display_name: string;
      category: string;
      description: string;
    }>;
    pattern_categories: string[];
    supported_actions: string[];
    content_categories?: Array<{
      name: string;
      display_name: string;
      description: string;
      default_action: string;
    }>;
  };
}
