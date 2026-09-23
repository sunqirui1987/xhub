import { getProviderLogoAndName, Providers, providerLogoMap } from "@/components/provider_info_helpers";
import milvusLogo from "../../public/assets/logos/milvus.svg";
import mongodbLogo from "../../public/assets/logos/mongodb.svg";
import postgresqlLogo from "../../public/assets/logos/postgresql.svg";
import s3VectorLogo from "../../public/assets/logos/s3_vector.png";
import valkeyLogo from "../../public/assets/logos/valkey.svg";
import { t } from "@/i18n";

export enum VectorStoreProviders {
  Bedrock = "Amazon Bedrock",
  S3Vectors = "Amazon S3 Vectors",
  PgVector = "PostgreSQL pgvector",
  VertexRagEngine = "Vertex AI RAG Engine",
  VertexAiSearch = "Vertex AI Search",
  OpenAI = "OpenAI",
  Azure = "Azure OpenAI",
  Milvus = "Milvus",
  MongoDB = "MongoDB (BETA)",
  Valkey = "Valkey",
}

export const vectorStoreProviderMap: Record<string, string> = {
  Bedrock: "bedrock",
  PgVector: "pg_vector",
  VertexRagEngine: "vertex_ai",
  VertexAiSearch: "vertex_ai/search_api",
  OpenAI: "openai",
  Azure: "azure",
  Milvus: "milvus",
  MongoDB: "mongodb",
  S3Vectors: "s3_vectors",
  Valkey: "valkey",
};

export const vectorStoreProviderLogoMap: Record<string, string> = {
  [VectorStoreProviders.Bedrock]: providerLogoMap[Providers.Bedrock] ?? "",
  [VectorStoreProviders.PgVector]: postgresqlLogo.src,
  [VectorStoreProviders.VertexRagEngine]: providerLogoMap[Providers.Vertex_AI] ?? "",
  [VectorStoreProviders.VertexAiSearch]: providerLogoMap[Providers.Vertex_AI] ?? "",
  [VectorStoreProviders.OpenAI]: providerLogoMap[Providers.OpenAI] ?? "",
  [VectorStoreProviders.Azure]: providerLogoMap[Providers.Azure] ?? "",
  [VectorStoreProviders.Milvus]: milvusLogo.src,
  [VectorStoreProviders.MongoDB]: mongodbLogo.src,
  [VectorStoreProviders.S3Vectors]: s3VectorLogo.src,
  [VectorStoreProviders.Valkey]: valkeyLogo.src,
};

// Define field types for provider-specific configurations
export interface VectorStoreFieldConfig {
  name: string;
  label: string;
  tooltip: string;
  placeholder?: string;
  required: boolean;
  type?: "text" | "password" | "select";
  options?: { value: string; label: string }[];
  initialValue?: string;
}

// Provider-specific field configurations
export const vectorStoreProviderFields: Record<string, VectorStoreFieldConfig[]> = {
  bedrock: [],
  pg_vector: [
    {
      name: "api_base",
      label: t("API Base"),
      tooltip: t("Enter the base URL of your deployed litellm-pgvector server (e.g., http://your-server:8000)"),
      placeholder: "http://your-deployed-server:8000",
      required: true,
      type: "text",
    },
    {
      name: "api_key",
      label: t("API Key"),
      tooltip: t("Enter the API key from your deployed litellm-pgvector server"),
      placeholder: "your-deployed-api-key",
      required: true,
      type: "password",
    },
  ],
  vertex_rag_engine: [],
  "vertex_ai/search_api": [
    {
      name: "vertex_project",
      label: t("Vertex Project"),
      tooltip: t("Google Cloud project ID that hosts the Vertex AI Search data store."),
      placeholder: "my-gcp-project-id",
      required: true,
      type: "text",
    },
    {
      name: "vertex_location",
      label: t("Vertex Location"),
      tooltip: t("Vertex AI Search data store location. Must be one of global, us, or eu."),
      required: true,
      type: "select",
      options: [
        { value: "global", label: t("global") },
        { value: "us", label: t("us") },
        { value: "eu", label: t("eu") },
      ],
      initialValue: "global",
    },
    {
      name: "vertex_collection_id",
      label: t("Collection ID (optional)"),
      tooltip: t("Discovery Engine collection ID. Leave blank to use the default collection."),
      placeholder: t("e.g. my-custom-collection"),
      required: false,
      type: "text",
    },
    {
      name: "vertex_engine_id",
      label: t("Engine ID (optional)"),
      tooltip:
        t("Search app (engine) ID. Required for website, healthcare, and connector-based data stores (Workspace, Slack, Jira, etc.) because these sources route search through an engine. Leave blank to query the data store directly."),
      placeholder: t("e.g. my-search-app_1234567890"),
      required: false,
      type: "text",
    },
  ],
  openai: [
    {
      name: "api_key",
      label: t("API Key"),
      tooltip: t("Enter your OpenAI API key"),
      placeholder: t("sk-..."),
      required: true,
      type: "password",
    },
  ],
  azure: [
    {
      name: "api_key",
      label: t("API Key"),
      tooltip: t("Enter your Azure OpenAI API key"),
      placeholder: "your-azure-api-key",
      required: true,
      type: "password",
    },
    {
      name: "api_base",
      label: t("API Base"),
      tooltip: t("Enter your Azure OpenAI endpoint (e.g., https://your-resource.openai.azure.com/)"),
      placeholder: "https://your-resource.openai.azure.com/",
      required: true,
      type: "text",
    },
  ],
  milvus: [
    {
      name: "api_key",
      label: t("API Key"),
      tooltip:
        t("To obtain a token, you should use a colon (:) to concatenate the username and password that you use to access your Milvus instance (e.g., username:password)"),
      placeholder: t("username:password or api key"),
      required: true,
      type: "password",
    },
    {
      name: "api_base",
      label: t("API Base"),
      tooltip: t("Enter your Milvus endpoint (e.g., https://your-milvus-endpoint.com/)"),
      placeholder: "https://your-milvus-endpoint.com/",
      required: true,
      type: "text",
    },
    {
      name: "embedding_model",
      label: t("Embedding Model"),
      tooltip: t("Select the embedding model to use"),
      placeholder: "text-embedding-3-small",
      required: true,
      type: "select",
    },
  ],
  mongodb: [
    {
      name: "api_base",
      label: t("Sidecar URL"),
      tooltip: t("Use HTTPS for a remote sidecar, or HTTP with a loopback IP for a sidecar on the same host or Pod"),
      placeholder: "http://127.0.0.1:8080",
      required: true,
      type: "text",
    },
    {
      name: "api_key",
      label: t("Sidecar API Key"),
      tooltip: t("The MONGODB_SIDECAR_API_KEY configured in your MongoDB sidecar"),
      placeholder: t("Enter sidecar API key"),
      required: true,
      type: "password",
    },
    {
      name: "mongodb_database",
      label: t("Database"),
      tooltip: t("The MongoDB database holding the collection you want to search"),
      placeholder: "sample_mflix",
      required: true,
      type: "text",
    },
    {
      name: "mongodb_collection",
      label: t("Collection"),
      tooltip: t("The collection your MongoDB Vector Search index was built on"),
      placeholder: "embedded_movies",
      required: true,
      type: "text",
    },
    {
      name: "embedding_model",
      label: t("Embedding Model"),
      tooltip:
        t("The embedding model on this proxy that created the vectors already stored in your collection. LiteLLM embeds every search query with it, so it must be the same model. A different model of the same size will not error, it will just return wrong results. Add it under Models first if it is not listed"),
      placeholder: "text-embedding-3-small",
      required: true,
      type: "select",
    },
    {
      name: "mongodb_embedding_field",
      label: t("Vector Field Name"),
      tooltip:
        t("The field in each document that holds its embedding. It must match the path your MongoDB Vector Search index was created on (default: embedding)"),
      placeholder: t("embedding"),
      required: false,
      type: "text",
      initialValue: "embedding",
    },
    {
      name: "mongodb_text_field",
      label: t("Text Field"),
      tooltip:
        t("The field in each document that holds its readable text. LiteLLM returns this text in search results, and it accepts a dotted path such as metadata.body (default: text)"),
      placeholder: t("text"),
      required: false,
      type: "text",
      initialValue: "text",
    },
    {
      name: "mongodb_num_candidates",
      label: t("Candidates Considered"),
      tooltip:
        "How many nearest neighbours MongoDB examines before returning the top results. Higher is more accurate and slower. Leave blank to let LiteLLM scale it with the requested result count",
      placeholder: "100",
      required: false,
      type: "text",
    },
  ],
  valkey: [
    {
      name: "valkey_host",
      label: t("Valkey Host"),
      tooltip: t("Hostname or IP of your Valkey server, without redis:// or a port (e.g. my-valkey.example.com)"),
      placeholder: "my-valkey.example.com",
      required: true,
      type: "text",
    },
    {
      name: "valkey_port",
      label: t("Valkey Port"),
      tooltip: t("Port your Valkey server listens on. Leave as 6379 unless you changed it"),
      placeholder: "6379",
      required: false,
      type: "text",
      initialValue: "6379",
    },
    {
      name: "valkey_password",
      label: t("Valkey Password"),
      tooltip: t("Password used to log in to your Valkey server. Leave blank if it has no password"),
      required: false,
      type: "password",
    },
    {
      name: "valkey_ssl",
      label: t("Use TLS"),
      tooltip:
        t("Set to true if your Valkey server requires an encrypted (TLS) connection, for example AWS ElastiCache with in-transit encryption turned on"),
      required: false,
      type: "select",
      options: [
        { value: "false", label: t("false") },
        { value: "true", label: t("true") },
      ],
      initialValue: "false",
    },
    {
      name: "embedding_model",
      label: t("Embedding Model"),
      tooltip:
        t("The embedding model on this proxy that was used to create the embeddings already stored in your Valkey index. LiteLLM uses it to embed each search query, so it must be the same model or results will be wrong. Add it under Models first if it is not listed"),
      placeholder: "text-embedding-3-small",
      required: true,
      type: "select",
    },
    {
      name: "valkey_text_field",
      label: t("Text Field"),
      tooltip:
        t("The field in each stored document that holds its readable text. LiteLLM returns this text in search results. Must match how your documents were stored (default: text)"),
      placeholder: t("text"),
      required: false,
      type: "text",
      initialValue: "text",
    },
    {
      name: "valkey_embedding_field",
      label: t("Vector Field Name"),
      tooltip:
        t("The field in each stored document that holds its embedding. LiteLLM searches against this field, so it must match the field your index was created on (default: embedding)"),
      placeholder: t("embedding"),
      required: false,
      type: "text",
      initialValue: "embedding",
    },
  ],
  s3_vectors: [
    {
      name: "vector_bucket_name",
      label: t("Vector Bucket Name"),
      tooltip: t("S3 bucket name for vector storage (will be auto-created if it doesn't exist)"),
      placeholder: "my-vector-bucket",
      required: true,
      type: "text",
    },
    {
      name: "index_name",
      label: t("Index Name"),
      tooltip: t("Name for the vector index (optional, will be auto-generated if not provided)"),
      placeholder: "my-vector-index",
      required: false,
      type: "text",
    },
    {
      name: "aws_region_name",
      label: t("AWS Region"),
      tooltip: t("AWS region where the S3 bucket is located (e.g., us-west-2)"),
      placeholder: "us-west-2",
      required: true,
      type: "text",
    },
    {
      name: "embedding_model",
      label: t("Embedding Model"),
      tooltip: t("Select the embedding model to use for vector generation"),
      placeholder: "text-embedding-3-small",
      required: true,
      type: "select",
    },
  ],
};

export const getVectorStoreProviderLogoAndName = (providerValue: string): { logo: string; displayName: string } => {
  const enumKey = Object.keys(vectorStoreProviderMap).find(
    (key) => vectorStoreProviderMap[key].toLowerCase() === providerValue.toLowerCase(),
  );
  if (!enumKey) {
    return getProviderLogoAndName(providerValue);
  }
  const displayName = VectorStoreProviders[enumKey as keyof typeof VectorStoreProviders];
  return { logo: vectorStoreProviderLogoMap[displayName], displayName };
};

export const getProviderSpecificFields = (providerValue: string): VectorStoreFieldConfig[] => {
  return vectorStoreProviderFields[providerValue] || [];
};
