import type { VideoModelPrice, VideoModelPrices } from "@/types";

export const DEFAULT_SEEDANCE_VIDEO_MODELS = [
  "seedance-2.0",
  "seedance-2.0-fast",
  "seedance-2.0-mini",
] as const;

export const DEFAULT_LTX_VIDEO_MODELS = ["ltx-2.3-pro", "ltx-2.3-fast"] as const;

export const DEFAULT_HAPPYHORSE_VIDEO_MODELS = ["happy-horse-1.1"] as const;

export const VIDEO_MODEL_PRICE_RESOLUTIONS = [
  "480p",
  "720p",
  "1080p",
  "1440p",
  "2160p",
] as const;

export type VideoModelPriceResolution =
  (typeof VIDEO_MODEL_PRICE_RESOLUTIONS)[number];

const VIDEO_MODEL_SUPPORTED_RESOLUTIONS: Record<
  string,
  readonly VideoModelPriceResolution[]
> = {
  "seedance-2.0": ["480p", "720p", "1080p"],
  "seedance-2.0-fast": ["480p", "720p"],
  "seedance-2.0-mini": ["480p", "720p"],
  "ltx-2.3-pro": ["1080p", "1440p", "2160p"],
  "ltx-2.3-fast": ["1080p", "1440p", "2160p"],
  "happy-horse-1.1": ["720p", "1080p"],
};

export const videoModelsForPricingPlatform = (
  platform: string,
): readonly string[] =>
  platform === "happyhorse"
    ? DEFAULT_HAPPYHORSE_VIDEO_MODELS
    : platform === "ltx"
    ? DEFAULT_LTX_VIDEO_MODELS
    : platform === "seedance"
      ? DEFAULT_SEEDANCE_VIDEO_MODELS
      : [];

export const supportedResolutionsForVideoModel = (
  platform: string,
  model: string,
): readonly VideoModelPriceResolution[] => {
  const normalizedModel = model.trim().toLowerCase();
  const configured = VIDEO_MODEL_SUPPORTED_RESOLUTIONS[normalizedModel];
  if (configured) {
    return configured;
  }
  return platform === "ltx"
    ? ["1080p", "1440p", "2160p"]
    : platform === "happyhorse"
      ? ["720p", "1080p"]
    : ["480p", "720p", "1080p"];
};

export const videoModelSupportsResolution = (
  platform: string,
  model: string,
  resolution: VideoModelPriceResolution,
): boolean => supportedResolutionsForVideoModel(platform, model).includes(resolution);

export type VideoModelPriceInput = number | string | null;

export interface VideoModelPriceRow {
  model: string;
  price_480p: VideoModelPriceInput;
  price_720p: VideoModelPriceInput;
  price_1080p: VideoModelPriceInput;
  price_1440p: VideoModelPriceInput;
  price_2160p: VideoModelPriceInput;
}

export type VideoModelPriceRowValidationError =
  | { code: "modelRequired"; row: number }
  | { code: "duplicateModel"; row: number; model: string }
  | { code: "invalidPrice"; row: number; model: string }
  | { code: "priceRequired"; row: number; model: string };

export const supportsVideoModelPricingPlatform = (
  platform: string,
): boolean => platform === "seedance" || platform === "ltx" || platform === "happyhorse";

export const videoModelPricePlaceholder = (platform: string): string =>
  videoModelsForPricingPlatform(platform)[0] ?? "video-model";

const resolutionFields = [
  ["price_480p", "480p"],
  ["price_720p", "720p"],
  ["price_1080p", "1080p"],
  ["price_1440p", "1440p"],
  ["price_2160p", "2160p"],
] as const;

const emptyPrice = (value: VideoModelPriceInput): boolean =>
  value === null || value === "";

const parsePrice = (value: VideoModelPriceInput): number | null => {
  if (emptyPrice(value)) {
    return null;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : null;
};

export const createVideoModelPriceRow = (
  model = "",
  price: VideoModelPrice = {},
): VideoModelPriceRow => ({
  model,
  price_480p: price["480p"] ?? null,
  price_720p: price["720p"] ?? null,
  price_1080p: price["1080p"] ?? null,
  price_1440p: price["1440p"] ?? null,
  price_2160p: price["2160p"] ?? null,
});

export const createDefaultVideoModelPriceRows = (
  platform = "seedance",
): VideoModelPriceRow[] =>
  videoModelsForPricingPlatform(platform).map((model) =>
    createVideoModelPriceRow(model),
  );

export const videoModelPricesToRows = (
  prices: VideoModelPrices | null | undefined,
): VideoModelPriceRow[] =>
  Object.entries(prices ?? {}).map(([model, price]) =>
    createVideoModelPriceRow(model, price),
  );

export const validateVideoModelPriceRows = (
  rows: VideoModelPriceRow[],
  platform = "seedance",
): VideoModelPriceRowValidationError | null => {
  const models = new Set<string>();

  for (const [index, row] of rows.entries()) {
    const model = row.model.trim();
    if (!model) {
      return { code: "modelRequired", row: index + 1 };
    }

    const normalizedModel = model.toLowerCase();
    if (models.has(normalizedModel)) {
      return {
        code: "duplicateModel",
        row: index + 1,
        model: normalizedModel,
      };
    }
    models.add(normalizedModel);

    let configuredPrices = 0;
    for (const [field, resolution] of resolutionFields) {
      if (!videoModelSupportsResolution(platform, normalizedModel, resolution)) {
        continue;
      }
      const value = row[field];
      if (emptyPrice(value)) {
        continue;
      }
      const parsed = Number(value);
      if (!Number.isFinite(parsed) || parsed < 0) {
        return {
          code: "invalidPrice",
          row: index + 1,
          model: normalizedModel,
        };
      }
      configuredPrices += 1;
    }

    if (configuredPrices === 0) {
      return {
        code: "priceRequired",
        row: index + 1,
        model: normalizedModel,
      };
    }
  }

  return null;
};

export const videoModelPriceRowsToPrices = (
  rows: VideoModelPriceRow[],
  platform = "seedance",
): VideoModelPrices => {
  const prices: VideoModelPrices = {};

  for (const row of rows) {
    const model = row.model.trim().toLowerCase();
    if (!model) {
      continue;
    }

    const card: VideoModelPrice = {};
    for (const [field, resolution] of resolutionFields) {
      if (!videoModelSupportsResolution(platform, model, resolution)) {
        continue;
      }
      const price = parsePrice(row[field]);
      if (price !== null) {
        card[resolution] = price;
      }
    }
    if (Object.keys(card).length > 0) {
      prices[model] = card;
    }
  }

  return prices;
};

export const videoModelPricesPayloadForPlatform = (
  platform: string,
  rows: VideoModelPriceRow[],
): VideoModelPrices | undefined =>
  supportsVideoModelPricingPlatform(platform)
    ? videoModelPriceRowsToPrices(rows, platform)
    : undefined;
