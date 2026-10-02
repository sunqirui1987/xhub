import { t } from "@/i18n";
// Define the available test modes
export const TEST_MODES = [
  { value: "chat", get label() { return t("Chat - /chat/completions"); } },
  { value: "completion", get label() { return t("Completion - /completions"); } },
  { value: "embedding", get label() { return t("Embedding - /embeddings"); } },
  { value: "audio_speech", get label() { return t("Audio Speech - /audio/speech"); } },
  { value: "audio_transcription", get label() { return t("Audio Transcription - /audio/transcriptions"); } },
  { value: "image_generation", get label() { return t("Image Generation - /images/generations"); } },
  { value: "image_edit", get label() { return t("Image Edit - /images/edits"); } },
  { value: "video_generation", get label() { return t("Video Generation - /videos"); } },
  { value: "rerank", get label() { return t("Rerank - /rerank"); } },
  { value: "realtime", get label() { return t("Realtime - /realtime"); } },
  { value: "batch", get label() { return t("Batch - /batch"); } },
  { value: "ocr", get label() { return t("OCR - /ocr"); } },
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
