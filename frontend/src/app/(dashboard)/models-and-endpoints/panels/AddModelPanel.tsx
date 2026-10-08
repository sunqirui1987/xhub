"use client";
import ModelEditor from "@/components/add_model/ModelEditor";
export default function AddModelPanel({
  onSaved,
  onCancel,
  initialCatalogId,
}: {
  onSaved?: () => void;
  onCancel?: () => void;
  initialCatalogId?: string;
}) {
  return <ModelEditor onSaved={onSaved} onCancel={onCancel} initialCatalogId={initialCatalogId} />;
}
