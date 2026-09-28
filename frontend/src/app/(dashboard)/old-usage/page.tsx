"use client";
import { useEffect, useState } from "react";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { apiClient } from "@/components/networking";
export default function OldUsagePage() {
  const { accessToken } = useAuthorized();
  const [text, setText] = useState("");
  const [ready, setReady] = useState(false);
  useEffect(() => {
    if (!accessToken) return;
    apiClient.get("/global/spend", { accessToken }).then((data) => setText(JSON.stringify(data))).catch((error: unknown) => setText(error instanceof Error ? error.message : "用量读取失败")).finally(() => setReady(true));
  }, [accessToken]);
  return (<div className="p-6"><h1 className="text-2xl font-semibold">历史用量</h1>{ready ? <p data-testid="old-usage-body">{text}</p> : <p>正在读取用量</p>}</div>);
}
