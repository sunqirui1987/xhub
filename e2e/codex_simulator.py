#!/usr/bin/env python3
"""隔离浏览器调用的 Codex 模拟入口，复用真实数据集的三轮请求与逐轮对账。"""
import json
import sys
from real_dataset import Dataset, load_manifest


def main():
    """用途：从标准输入读取网关、身份与临时目录，输出脱敏会话证据；无参数/返回值。

    Playwright 调用；凭据不进入 argv，真实请求或对账失败时退出非零，临时目录及数据库由浏览器 runner 清理。
    """
    config = json.load(sys.stdin)
    dataset = Dataset(config["gateway"], config["directory"], load_manifest(), logger=lambda _: None)
    dataset.admin = config["admin"]
    print(json.dumps(dataset.codex_conversation(config["key"])))


if __name__ == "__main__":
    main()
