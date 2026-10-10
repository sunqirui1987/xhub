/** guardCallbacks 将响应回调绑定到当前请求身份。
 * 参数 callbacks：状态更新函数；active：身份检查；返回同类型回调集合。
 * 调用：对话和对比；切换、停止、卸载后的迟到响应被忽略，即使客户端未及时响应取消。 */
export function guardCallbacks<T extends Record<string, (...args: any[]) => any>>(
  callbacks: T,
  active: () => boolean,
): T {
  return Object.fromEntries(
    Object.entries(callbacks).map(([key, callback]) => [
      key,
      (...args: any[]) => {
        if (active()) return callback(...args);
      },
    ]),
  ) as T;
}
