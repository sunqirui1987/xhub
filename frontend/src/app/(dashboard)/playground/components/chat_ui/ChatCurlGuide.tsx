"use client";
import { useEffect, useState } from "react";
import { CurlRequestGuide } from "./CurlRequestGuide";
import { playgroundCurlBody, type CurlBodyOptions } from "./playgroundCurlBody";
import { createChatMultimodalMessage } from "./ChatImageUtils";
import { createMultimodalMessage } from "./ResponsesImageUtils";

/** 为聊天、图片及音频表单展示实时 curl；参数为正文选项、标签和当前附件，返回统一教程。
 * 调用方 ChatUI；多模态附件异步编码，切换草稿时忽略旧读取；多张编辑图片按界面逐张发送。
 * 不读取密钥、不发送网络请求，文件上传示例只包含本地文件名，编码失败显示无效正文提示。 */
export function ChatCurlGuide({
  options,
  tags,
  image,
  files,
}: {
  options: CurlBodyOptions & { model: string };
  tags: string[];
  image?: File | null;
  files?: File[];
}) {
  const [attachment, setAttachment] = useState<{
    file: File;
    input: string;
    message: { role: string; content: unknown };
  } | null>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    setFailed(false);
    if (image) {
      const encode =
        options.endpoint.protocol === "openai-responses" ? createMultimodalMessage : createChatMultimodalMessage;
      void encode(options.input, image)
        .then((message) => {
          if (active) setAttachment({ file: image, input: options.input, message });
        })
        .catch(() => {
          if (active) setFailed(true);
        });
    }
    return () => {
      active = false;
    };
  }, [image, options.input, options.endpoint.protocol]);
  const messages = [...options.messages];
  if (options.input.trim() || image)
    messages.push(
      attachment && attachment.file === image && attachment.input === options.input
        ? attachment.message
        : { role: "user", content: options.input },
    );
  const body = playgroundCurlBody({ ...options, messages });
  const multipart =
    options.endpoint.path.endsWith("/images/edits") ||
    ["openai-audio-transcription", "openai-audio-translation"].includes(options.endpoint.protocol);
  const names = multipart
    ? files?.length
      ? files.map((file) => file.name)
      : [options.endpoint.path.endsWith("/images/edits") ? "input.png" : "audio.mp3"]
    : [""];
  return (
    <>
      {names.map((name, index) => (
        <CurlRequestGuide
          key={index}
          endpoint={options.endpoint}
          model={options.model}
          base={options.base}
          body={
            failed || (image && (attachment?.file !== image || attachment.input !== options.input))
              ? ""
              : JSON.stringify(body)
          }
          multipart={multipart}
          files={multipart ? { [options.endpoint.path.endsWith("/images/edits") ? "image" : "file"]: name } : undefined}
          headers={tags.length ? { "x-litellm-tags": tags.join(",") } : undefined}
        />
      ))}
    </>
  );
}
