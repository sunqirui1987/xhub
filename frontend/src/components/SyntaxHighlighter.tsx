import PrismLight from "react-syntax-highlighter/dist/esm/prism-light";
import bash from "react-syntax-highlighter/dist/esm/languages/prism/bash";
import python from "react-syntax-highlighter/dist/esm/languages/prism/python";
import javascript from "react-syntax-highlighter/dist/esm/languages/prism/javascript";
import typescript from "react-syntax-highlighter/dist/esm/languages/prism/typescript";
import jsx from "react-syntax-highlighter/dist/esm/languages/prism/jsx";
import tsx from "react-syntax-highlighter/dist/esm/languages/prism/tsx";
import json from "react-syntax-highlighter/dist/esm/languages/prism/json";
import yaml from "react-syntax-highlighter/dist/esm/languages/prism/yaml";
import markdown from "react-syntax-highlighter/dist/esm/languages/prism/markdown";
import sql from "react-syntax-highlighter/dist/esm/languages/prism/sql";
import go from "react-syntax-highlighter/dist/esm/languages/prism/go";
import css from "react-syntax-highlighter/dist/esm/languages/prism/css";

// 代码块共用有限语言注册；Light 内置 markup/clike，未知语言按纯文本渲染。
// 直接导入避免入口同时加载完整 Prism、Highlight.js 和全部主题。
for (const grammar of [bash, python, javascript, typescript, jsx, tsx, json, yaml, markdown, sql, go, css]) {
  PrismLight.registerLanguage("", grammar);
}
PrismLight.alias("bash", ["shell", "sh"]);
export default PrismLight;
