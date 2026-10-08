"use client";

// Legacy URL stays readable, while the current identity model has no tag CRUD.
export default function Page() {
  return <section className="p-6"><h1 className="text-xl font-semibold">标签</h1><p className="mt-4">当前权限模型不支持标签管理。请通过组织、团队和项目管理访问范围。</p></section>;
}
