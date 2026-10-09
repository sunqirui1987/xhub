import { redirect } from "next/navigation";

/** 将历史供应商目录地址导向统一模型管理页。
 * 参数：无；返回：Next.js 重定向；旧 URL 不再选择隐式供应商，用户从已保存凭据导入目录。 */
export default function ModelCatalogPage() {
  redirect("/models-and-endpoints");
}
