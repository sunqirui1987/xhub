import { expect, test, type Page } from "@playwright/test";
import { GATEWAY, UPSTREAM, loginAdmin, sessionBearer, stableGoto, t, watchGateway } from "./helpers";

/** 选择后台提供的供应商；page 为已打开弹窗浏览器，label 为显示名，返回完成 Promise；仅变更草稿。 */
async function choose(page: Page, label: string) {
  const input=page.getByRole("combobox",{name:t("Provider type")});
  await input.click();
  await input.fill("");
  await input.pressSequentially(label);
  await page.getByRole("option").filter({has:page.getByText(label,{exact:true})}).click();
  // 下拉会保留隐藏节点供下一次搜索复用；等待其不可见，防止后续选择沿用关闭动画中的状态。
  await expect(page.getByRole("listbox")).toBeHidden();
}
/** 打开模型提供商弹窗；page 已登录，返回弹窗定位器；供创建流程调用，不写数据库。 */
async function openAdd(page:Page) {
  await stableGoto(page,"/models-and-endpoints");
  await page.getByRole("tab",{name:"模型提供商",exact:true}).click();
  await page.getByRole("button",{name:"添加提供商",exact:true}).click();
  return page.getByRole("dialog",{name:t("Add model provider")});
}
/** 从列表删除 name 凭据；page 已登录，返回完成 Promise；UI 确认后验证行消失，供用例清理。 */
async function deleteProvider(page:Page,name:string) {
  await stableGoto(page,"/models-and-endpoints");
  await page.getByRole("tab",{name:"模型提供商",exact:true}).click();
  await page.getByTestId("credential-actions-"+name).click();
  await page.getByTestId("credential-action-delete").click();
  const dialog=page.getByRole("dialog");
  await dialog.getByPlaceholder(name,{exact:true}).fill(name);
  await dialog.getByRole("button",{name:t("Delete"),exact:true}).click();
  await expect(page.getByTestId("credential-actions-"+name)).toHaveCount(0);
}

/** 前置真实浏览器、网关和数据库；验证专属字段切换、默认值、DeepSeek 创建编辑和删除。
 * 全流程只保存隔离测试凭据，无外部请求；显式删除，失败时测试 schema 自动清理。 */
test("provider forms switch and persist DeepSeek credentials",async({page})=>{
  const guard=watchGateway(page);await loginAdmin(page);const dialog=await openAdd(page);
  await expect(dialog.getByText(t("Select a provider to see its configuration fields."))).toBeVisible();
  await expect(dialog.getByLabel("OpenAI API Key")).toHaveCount(0);
  await choose(page,"OpenAI");
  await dialog.getByLabel(t("Provider name")).fill("e2e-form-deepseek");
  await dialog.getByLabel("OpenAI API Key").fill("old-secret");
  await dialog.getByLabel("OpenAI Organization ID").fill("old-organization");
  await choose(page,"Azure");
  await expect(dialog.getByLabel("API Version",{exact:true})).toBeVisible();
  await expect(dialog.getByLabel("Azure AD Token",{exact:true})).toBeVisible();
  await expect(dialog.getByLabel("API Base",{exact:true})).toHaveValue("");
  await expect(dialog.getByLabel("Azure API Key",{exact:true})).toHaveValue("");
  await page.screenshot({path:"../.e2e/provider-forms/azure.png"});
  await choose(page,"Vertex AI (Anthropic, Gemini, etc.)");
  await expect(dialog.getByLabel("Vertex Project",{exact:true})).toBeVisible();
  await expect(dialog.getByLabel("Vertex Location",{exact:true})).toBeVisible();
  await expect(dialog.getByLabel("Vertex Credentials",{exact:true})).toBeAttached();
  await choose(page,"Amazon Bedrock");
  await expect(dialog.getByLabel("AWS Region Name",{exact:true})).toBeVisible();
  await expect(dialog.getByLabel("AWS Secret Access Key",{exact:true})).toBeVisible();
  await choose(page,"Ollama");
  await expect(dialog.getByLabel("API Base",{exact:true})).toHaveValue("http://localhost:11434");
  const providerSearch=page.getByRole("combobox",{name:t("Provider type")});
  await providerSearch.click();await providerSearch.fill("ChatGPT Subscription");
  await expect(page.getByRole("option",{name:"ChatGPT Subscription",exact:true})).toHaveCount(0);
  await choose(page,"DeepSeek");
  await expect(dialog.getByLabel("API Base",{exact:true})).toHaveValue("https://api.deepseek.com");
  await expect(dialog.getByLabel("API Key",{exact:true})).toHaveValue("");
  await dialog.getByLabel("API Key",{exact:true}).fill("sk-fake");
  await page.screenshot({path:"../.e2e/provider-forms/deepseek.png"});
  await page.setViewportSize({width:390,height:844});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
  await page.screenshot({path:"../.e2e/provider-forms/mobile.png"});
  await page.setViewportSize({width:1440,height:1000});
  const saving=page.waitForResponse(r=>new URL(r.url()).pathname==="/credentials"&&r.request().method()==="POST");
  await dialog.getByRole("button",{name:t("Add model provider"),exact:true}).click();
  const saved=await saving;expect(saved.status(),await saved.text()).toBe(200);
  expect(saved.request().postDataJSON()).toMatchObject({credential_info:{custom_llm_provider:"deepseek",provider_id:"Deepseek"},credential_values:{api_base:"https://api.deepseek.com",api_key:"sk-fake"}});
  expect(saved.request().postDataJSON().credential_values).not.toHaveProperty("organization");
  await page.reload();await page.getByRole("tab",{name:"模型提供商",exact:true}).click();
  await page.getByTestId("credential-actions-e2e-form-deepseek").click();await page.getByTestId("credential-action-edit").click();
  const edit=page.getByRole("dialog",{name:t("Edit model provider")});
  await expect(edit.getByRole("combobox",{name:t("Provider type")})).toHaveValue("DeepSeek");
  await expect(edit.getByRole("combobox",{name:t("Provider type")})).toBeDisabled();
  await edit.getByLabel("API Base",{exact:true}).fill(UPSTREAM+"/v1");
  const updating=page.waitForResponse(r=>new URL(r.url()).pathname==="/credentials/e2e-form-deepseek"&&r.request().method()==="PATCH");
  await edit.getByRole("button",{name:t("Save model provider")}).click();expect((await updating).status()).toBe(200);
  await deleteProvider(page,"e2e-form-deepseek");guard.assertOk();
});

/** 前置真实浏览器到数据库与本地 OpenAI 兼容上游；验证表单标识、创建模型、连接与数据面结果。
 * 供应商和模型均经 UI 创建及删除；不依赖真实密钥，异常路径由隔离 schema 清理。 */
test("OpenAI compatible form saves a provider and invokes its model",async({page})=>{
  test.setTimeout(90_000);const guard=watchGateway(page);await loginAdmin(page);const name="e2e-form-compatible";
  const dialog=await openAdd(page);await choose(page,"OpenAI-Compatible Endpoints (Together AI, etc.)");
  await dialog.getByLabel(t("Provider name")).fill(name);await dialog.getByLabel("API Base",{exact:true}).fill(UPSTREAM+"/v1");
  await dialog.getByLabel("OpenAI API Key").fill("sk-fake");
  await dialog.getByRole("button",{name:t("Add model provider"),exact:true}).click();await expect(dialog).toHaveCount(0);
  await page.getByTestId("credential-actions-"+name).click();await page.getByTestId("credential-action-edit").click();
  const edit=page.getByRole("dialog",{name:t("Edit model provider")});
  await expect(edit.getByRole("combobox",{name:t("Provider type")})).toHaveValue("OpenAI-Compatible Endpoints (Together AI, etc.)");
  await edit.getByRole("button",{name:t("Cancel"),exact:true}).click();
  await page.getByRole("button",{name:t("pages.models.add"),exact:true}).click();
  const form=page.locator("form").filter({has:page.getByLabel("模型提供商 *")});
  await form.getByLabel("模型提供商 *").selectOption(name);
  const upstream=form.getByRole("combobox",{name:"上游模型 *"});await upstream.fill("gpt-4o-mini");
  await page.getByRole("option",{name:"gpt-4o-mini",exact:true}).click();
  await form.getByLabel("对外模型名称 *").fill(name);
  await form.getByRole("combobox",{name:t("Endpoint type"),exact:true}).click();await page.getByRole("option",{name:t("Chat"),exact:true}).click();
  await form.getByRole("combobox", { name: "上游接口协议", exact: true }).click();
  await page.getByRole("option", { name: "Chat Completions", exact: true }).click();
  await form.getByLabel("价格来源").selectOption("manual");
  await form.locator("#editor-input_cost_per_token").fill("0.15");await form.locator("#editor-output_cost_per_token").fill("0.60");
  const creating=page.waitForResponse(r=>new URL(r.url()).pathname==="/model/new"&&r.request().method()==="POST");
  await form.getByRole("button",{name:"添加模型",exact:true}).click();expect((await creating).status()).toBe(200);
  await page.reload();await stableGoto(page,"/models-and-endpoints");await page.getByRole("tab",{name:t("pages.models.all")}).click();
  await page.getByRole("button",{name,exact:true}).click();await page.getByRole("button",{name:"测试连接",exact:true}).click();
  await expect(page.getByRole("status").filter({hasText:"连接正常，模型已响应。"})).toBeVisible();
  const response=await page.request.post(GATEWAY+"/v1/chat/completions",{headers:{Authorization:"Bearer "+await sessionBearer(page)},data:{model:name,messages:[{role:"user",content:"provider-form-e2e"}]}});
  expect(response.status(),await response.text()).toBe(200);expect((await response.json()).choices[0].message.content).toBe("e2e-ok");
  await page.getByRole("button",{name:"删除",exact:true}).click();await page.getByRole("dialog").getByPlaceholder(name).fill(name);
  await page.getByRole("dialog").getByRole("button",{name:"删除",exact:true}).click();
  await expect(page.getByRole("button",{name,exact:true})).toHaveCount(0);await deleteProvider(page,name);guard.assertOk();
});

/** 前置真实浏览器、网关和本地任务上游；通过 UI 创建 Custom 凭据并显式选择七牛内容生成协议。
 * 验证保存、刷新回显和实际任务调用都使用用户填写的地址；模型与凭据经 UI 删除，schema 兜底清理。 */
test("Custom provider explicitly selects and invokes a Qiniu compatible transport",async({page})=>{
  test.setTimeout(90_000);const guard=watchGateway(page);await loginAdmin(page);
  const credential="e2e-custom-qiniu",name="e2e-custom-qiniu-model";
  const dialog=await openAdd(page);await choose(page,"Custom");
  await dialog.getByLabel(t("Provider name")).fill(credential);
  await dialog.getByLabel("API Base",{exact:true}).fill(UPSTREAM);await dialog.getByLabel("API Key",{exact:true}).fill("sk-fake");
  await dialog.getByRole("button",{name:t("Add model provider"),exact:true}).click();await expect(dialog).toHaveCount(0);
  await page.getByRole("button",{name:t("pages.models.add"),exact:true}).click();
  const form=page.locator("form").filter({has:page.getByLabel("模型提供商 *")});await form.getByLabel("模型提供商 *").selectOption(credential);
  const upstream=form.getByRole("combobox",{name:"上游模型 *"});
  await upstream.fill("qiniu/bytedance/doubao-seedance-2-0-260128");await upstream.press("Escape");
  const endpoint=form.getByRole("combobox",{name:t("Endpoint type"),exact:true});await expect(endpoint).toContainText("Ark Video");
  await form.getByLabel("对外模型名称 *").fill(name);await form.getByLabel("价格来源").selectOption("manual");
  await form.locator("#editor-output_cost_per_token").fill("10");
  const creating=page.waitForResponse(r=>new URL(r.url()).pathname==="/model/new"&&r.request().method()==="POST");
  await form.getByRole("button",{name:"添加模型",exact:true}).click();const saved=await creating;expect(saved.status(),await saved.text()).toBe(200);
  expect(saved.request().postDataJSON()).toMatchObject({litellm_params:{custom_llm_provider:"custom",litellm_credential_name:credential},model_info:{transport:"qiniu_contents_generation",endpoint_types:["bypass:ark-video"]}});
  await page.reload();await stableGoto(page,"/models-and-endpoints");await page.getByRole("tab",{name:t("pages.models.all")}).click();
  await page.getByRole("button",{name,exact:true}).click();await page.getByRole("button",{name:"编辑模型",exact:true}).click();
  await expect(upstream).toHaveValue("qiniu/bytedance/doubao-seedance-2-0-260128");await expect(endpoint).toContainText("Ark Video");
  const headers={Authorization:"Bearer "+await sessionBearer(page)};
  const queued=await page.request.post(GATEWAY+"/v3/contents/generations/tasks",{headers,data:{model:name,content:[{type:"text",text:"custom-qiniu-e2e"}]}});
  expect(queued.status(),await queued.text()).toBe(200);expect((await queued.json()).id).toMatch(/^ark-e2e-/);
  await form.getByRole("button",{name:"取消",exact:true}).click();await page.getByRole("button",{name:"删除",exact:true}).click();
  await page.getByRole("dialog",{name:"删除模型"}).getByPlaceholder(name,{exact:true}).fill(name);
  await page.getByRole("dialog",{name:"删除模型"}).getByRole("button",{name:/删除|确认/}).click();
  await deleteProvider(page,credential);guard.assertOk();
});
