import React from "react";
import { Loader2 } from "lucide-react";
import EmptyState from "./EmptyState";
import MessageBubble from "./MessageBubble";
import { Message } from "./types";
import { t } from "@/i18n";

interface MessageListProps {
  messages: Message[];
  isLoading: boolean;
  hasVariables: boolean;
  messagesEndRef: React.RefObject<HTMLDivElement | null>;
}

const MessageList: React.FC<MessageListProps> = ({ messages, isLoading, hasVariables, messagesEndRef }) => {
  return (
    <div className="flex-1 overflow-y-auto p-4 pb-0">
      {messages.length === 0 && <EmptyState hasVariables={hasVariables} />}

      {messages.map((message, index) => (
        <MessageBubble key={index} message={message} />
      ))}

      {isLoading && (
        <div className="flex justify-center items-center my-4">
          <Loader2 className="size-6 animate-spin text-muted-foreground" aria-label={t("Loading response")} />
        </div>
      )}
      <div ref={messagesEndRef} style={{ height: "1px" }} />
    </div>
  );
};

export default MessageList;
