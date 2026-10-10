/** 用途：逐项显示浏览器测试计划、开始和结果；由 Playwright 调用，无凭据或页面正文输出，报告文件仍由原有 reporter 保存。 */
class ChecklistReporter {
  /** 用途：接收完整测试树并编号；参数为配置及 suite，无返回；运行前公布待测项，标题用于业务核对。 */
  onBegin(config, suite) {
    this.tests = suite.allTests();
    for (const test of this.tests) this.print('待测', test);
  }
  /** 用途：输出稳定编号和业务标题；参数为状态及测试，无返回；仅输出测试定义，不打印请求或凭据。 */
  print(status, test) {
    const index = String(this.tests.indexOf(test) + 1).padStart(3, '0');
    const total = String(this.tests.length).padStart(3, '0');
    console.log('[浏览器' + status + ' ' + index + '/' + total + '] ' + test.title);
  }
  /** 用途：测试执行前立即显示测试内容；参数为测试，无返回；失败后的未执行项保留为待测。 */
  onTestBegin(test) { this.print('开始', test); }
  /** 用途：区分通过、失败、中断和跳过；参数为测试与结果，无返回；跳过不能显示为通过，原始错误由报告文件保存。 */
  onTestEnd(test, result) {
    const names = {passed: '通过', failed: '失败', timedOut: '超时', skipped: '未执行', interrupted: '中断'};
    this.print(names[result.status] || result.status, test);
  }
}
module.exports = ChecklistReporter;
