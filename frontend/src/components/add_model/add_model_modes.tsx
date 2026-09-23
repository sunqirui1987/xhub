import { t } from "@/i18n";
// Define the available test modes
export const TEST_MODES = [
  { value: "chat", label: t("Chat - /chat/completions") },
  { value: "completion", label: t("Completion - /completions") },
  { value: "embedding", label: t("Embedding - /embeddings") },
  { value: "audio_speech", label: t("Audio Speech - /audio/speech") },
  { value: "audio_transcription", label: t("Audio Transcription - /audio/transcriptions") },
  { value: "image_generation", label: t("Image Generation - /images/generations") },
  { value: "image_edit", label: t("Image Edit - /images/edits") },
  { value: "video_generation", label: t("Video Generation - /videos") },
  { value: "rerank", label: t("Rerank - /rerank") },
  { value: "realtime", label: t("Realtime - /realtime") },
  { value: "batch", label: t("Batch - /batch") },
  { value: "ocr", label: t("OCR - /ocr") },
];

// Define the available auto router routing strategies
export const AUTO_ROUTER_MODES = [
  { value: "simple-shuffle", label: t("Simple Shuffle - Random selection from available models") },
  { value: "least-busy", label: t("Least Busy - Route to model with lowest current load") },
  { value: "latency-based", label: t("Latency Based - Route to model with best response time") },
  { value: "cost-based", label: t("Cost Based - Route to most cost-effective model") },
  { value: "usage-based", label: t("Usage Based - Route based on historical usage patterns") },
  { value: "custom", label: t("Custom - Use custom routing logic defined in config") },
];
