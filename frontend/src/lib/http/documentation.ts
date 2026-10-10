/** 发出文档调试请求；参数为完整网关地址与浏览器请求选项，返回未经解析的响应。
 * 文档运行器调用以显示 JSON、SSE、二进制及非成功状态；不重试、不注入管理会话，网络错误传给调用方。 */
export function requestDocumentation(url: string, options: RequestInit): Promise<Response> {
  return fetch(url, options);
}
