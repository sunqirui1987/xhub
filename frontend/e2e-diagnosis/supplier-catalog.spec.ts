import {expect,test} from "@playwright/test";
import {GATEWAY,UPSTREAM,loginAdmin,sessionBearer,stableGoto,t} from "./helpers";

/** 前置真实浏览器、隔离后台及本地 Ark/Fal；创建 OpenAI 兼容目录连接、切换模型能力、保存刷新、任务实调和无效目录降级。
 * 验证专用接口默认折叠后可展开查看已选直通入口；不使用外部密钥，finally 删除模型和连接，schema 清理任务账单。 */
for (const catalogId of ["qiniu", "fennoai"]) {
test(`supplier directory ${catalogId} binds Seedance and Fal to implemented video capabilities`,async({page})=>{
 test.setTimeout(120_000);await loginAdmin(page);
 const headers={Authorization:"Bearer "+await sessionBearer(page)};
 const credential="e2e-directory-video-"+catalogId;const ids:string[]=[];
 try {
  await stableGoto(page,"/model-providers");
  await page.getByRole("button",{name:"添加提供商",exact:true}).click();
  const dialog=page.getByRole("dialog");
  const type=dialog.getByRole("combobox",{name:t("Provider type")});
  await type.fill("OpenAI");
  await page.getByRole("option").filter({has:page.getByText("OpenAI",{exact:true})}).click();
  await dialog.getByLabel(t("Provider name")).fill(credential);
  await dialog.getByLabel("供应商目录 ID").fill(catalogId);
  await dialog.getByLabel("API Base",{exact:true}).fill(UPSTREAM);
  await dialog.getByLabel("OpenAI API Key",{exact:true}).fill("sk-fake");
  const creating=page.waitForResponse(r=>new URL(r.url()).pathname==="/credentials"&&r.request().method()==="POST");
  await dialog.getByRole("button",{name:t("Add model provider"),exact:true}).click();
  const created=await creating;expect(created.status(),await created.text()).toBe(200);
  expect(created.request().postDataJSON().credential_info.catalog_id).toBe(catalogId);
  expect(created.request().postDataJSON().credential_values).not.toHaveProperty("catalog_id");
  await expect(dialog).toHaveCount(0);
  for (const spec of [
   {model:"bytedance/doubao-seedance-2-0-260128",label:"Ark Video · Seedance",transport:"qiniu_contents_generation",path:"/v3/contents/generations/tasks",key:"id"},
   {model:"byteplus/seedance-2.0/text-to-video",label:"FAL · Dreamina Seedance 2.0",transport:"qiniu_fal_dreamina_20",path:"/queue/byteplus/seedance-2.0/text-to-video",key:"request_id"},
  ]) {
   const name="e2e-directory/"+spec.transport+":latest";
   await stableGoto(page,"/models-and-endpoints");
   await page.getByRole("button",{name:t("pages.models.add"),exact:true}).click();
   const form=page.locator("form").filter({has:page.getByLabel("模型提供商 *")});
   await form.getByLabel("模型提供商 *").selectOption(credential);
   const upstream=form.getByRole("combobox",{name:"上游模型 *"});
   await upstream.fill(spec.model);await upstream.press("Escape");
   const protocol=form.getByRole("combobox",{name:"上游接口协议",exact:true});
   if(catalogId==="qiniu") await expect(protocol).toContainText(spec.label);
   await protocol.click();
   await expect(page.getByRole("listbox").getByRole("option",{name:"OpenAI · Chat Completions",exact:true})).toBeVisible();
   await page.getByRole("listbox").getByRole("option",{name:new RegExp(spec.label)}).click();
   const published=form.getByRole("region",{name:"XHub 对外接口"});
   const toggle=published.getByRole("button",{name:"XHub 对外接口",exact:true});
   await expect(toggle).toHaveAttribute("aria-expanded","false");
   await toggle.click();
   await expect(form.getByRole("region",{name:"XHub 对外接口"}).getByRole("checkbox")).toBeChecked();
   // 普通型号恢复 OpenAI 默认；FAL 仍可选择，错误型号明确解释并由保存接口校验。
   await upstream.fill("unimplemented-video");await upstream.press("Escape");
   await expect(protocol).toContainText("OpenAI · Chat Completions");
   await protocol.click();
   await page.getByRole("listbox").getByRole("option",{name:"FAL · Dreamina Seedance 2.0",exact:true}).click();
   await expect(form.getByRole("alert")).toContainText("尚未实现当前模型或目录的调用");
   await upstream.fill(spec.model);await upstream.press("Escape");
   {
    await protocol.click();await page.getByRole("listbox").getByRole("option",{name:new RegExp(spec.label)}).click();
   }
   await expect(protocol).toContainText(spec.label);
   await form.getByLabel("对外模型名称 *").fill(name);
   await form.getByLabel("价格来源").selectOption("manual");
   await form.locator("#editor-output_cost_per_token").fill("1");
   const saving=page.waitForResponse(r=>new URL(r.url()).pathname==="/model/new"&&r.request().method()==="POST",{timeout:15_000});
   await form.getByRole("button",{name:"添加模型",exact:true}).click();
   const saved=await saving;expect(saved.status(),await saved.text()).toBe(200);
   const result=await saved.json();ids.push(result.model_info.id);
   expect(result.model_info).toMatchObject({catalog_id:catalogId,transport:spec.transport});
   await page.reload();
   const stored=await page.request.get(GATEWAY+"/v2/model/info?modelId="+result.model_info.id,{headers});
   expect((await stored.json()).data[0].model_info.catalog_id).toBe(catalogId);
   const run=await page.request.post(GATEWAY+spec.path,{headers,data:{model:name,prompt:"test video",content:[{type:"text",text:"test video"}]}});
   expect(run.status(),await run.text()).toBe(200);const task=await run.json();expect(task[spec.key]).toBeTruthy();
   const poll=await page.request.get(GATEWAY+(spec.key==="id"?spec.path+"/"+task.id:"/queue/byteplus/seedance-2.0/requests/"+task.request_id),{headers});
   expect(poll.status(),await poll.text()).toBe(200);expect(await poll.text()).toContain("video_url");
  }
 } finally {
  // 清理使用独立超时，防止原始失败被已耗尽的用例时间覆盖；schema 仍兜底清理。
  test.setTimeout(150_000);
  for(const id of ids) expect((await page.request.post(GATEWAY+"/model/delete",{headers,data:{id}})).status()).toBe(200);
  await page.request.delete(GATEWAY+"/credentials/"+credential,{headers});
 }
});
}
