import { parseAsString, useQueryState } from "nuqs";
import { useCallback } from "react";

export interface ModelDetailRouting {
  modelId: string | null;
  openModel: (id: string) => void;
  close: () => void;
}

export function useModelDetailRouting(): ModelDetailRouting {
  const [modelId, setModelId] = useQueryState("model", parseAsString.withOptions({ history: "push" }));

  const openModel = useCallback(
    (id: string) => {
      void setModelId(id);
    },
    [setModelId],
  );

  const close = useCallback(() => {
    void setModelId(null);
  }, [setModelId]);

  return { modelId, openModel, close };
}

export interface ModelGroupFilterRouting {
  modelGroup: string | null;
  setModelGroup: (modelGroup: string | null) => void;
}

export function useModelGroupFilterRouting(): ModelGroupFilterRouting {
  const [modelGroup, setParam] = useQueryState("model_group", parseAsString);

  const setModelGroup = useCallback(
    (next: string | null) => {
      void setParam(next);
    },
    [setParam],
  );

  return { modelGroup, setModelGroup };
}
