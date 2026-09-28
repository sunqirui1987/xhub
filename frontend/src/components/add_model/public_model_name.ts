export interface NameMapping {
  public_name: string;
  litellm_model: string;
}

/**
 * A public name the user typed themselves. The placeholder "custom" and a name that still
 * matches the provider model are the form's own defaults, so a later model-id update may replace them.
 */
export const typedPublicName = (mapping: NameMapping | undefined): string | undefined => {
  if (!mapping) return undefined;
  const pub = mapping.public_name.trim();
  if (pub === "" || pub === "custom") return undefined;
  if (pub === mapping.litellm_model) return undefined;
  if (mapping.litellm_model === `azure/${pub}`) return undefined;
  return mapping.public_name;
};

export const withPublicName = (
  mapping: NameMapping | undefined,
  fallback: string,
  litellmModel: string,
): NameMapping => ({
  public_name: typedPublicName(mapping) ?? fallback,
  litellm_model: litellmModel,
});
