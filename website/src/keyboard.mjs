/** 处理文档搜索快捷键；参数为键盘事件、当前焦点、原生对话框和打开回调，返回是否已处理；页面键盘监听调用，关闭时恢复原生焦点，编辑正文时保留斜杠输入。 */
export function handleSearchKey(event, activeElement, dialog, openSearch) {
  if (event.key === "Escape" && dialog.open) {
    // 显式关闭保证不同浏览器的 Escape 行为一致，不依赖原生取消事件的默认动作。
    event.preventDefault();
    dialog.close();
    return true;
  }
  const editing = /^(INPUT|TEXTAREA|SELECT)$/.test(activeElement?.tagName || "") || activeElement?.isContentEditable;
  if ((event.key === "/" && !editing) || ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k")) {
    event.preventDefault();
    openSearch();
    return true;
  }
  return false;
}
