import type { VideoModelPrice, VideoModelPrices } from "@/types";

export const DEFAULT_SEEDANCE_VIDEO_MODELS = [
  "doubao-seedance-2-0-pro",
  "doubao-seedance-2-0-fast",
] as const;

export type VideoModelPriceInput = number | string | null;

export interface VideoModelPriceRow {
  model: string;
  price_480p: VideoModelPriceInput;
  price_720p: VideoModelPriceInput;
  price_1080p: VideoModelPriceInput;
}

export type VideoModelPriceRowValidationError =
  | { code: "modelRequired"; row: number }
  | { code: "duplicateModel"; row: number; model: string }
  | { code: "invalidPrice"; row: number; model: string }
  | { code: "priceRequired"; row: number; model: string };

const resolutionFields = [
  ["price_480p", "480p"],
  ["price_720p", "720p"],
  ["price_1080p", "1080p"],
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
});

export const createDefaultSeedanceVideoModelPriceRows = (): VideoModelPriceRow[] =>
  DEFAULT_SEEDANCE_VIDEO_MODELS.map((model) => createVideoModelPriceRow(model));

export const videoModelPricesToRows = (
  prices: VideoModelPrices | null | undefined,
): VideoModelPriceRow[] =>
  Object.entries(prices ?? {}).map(([model, price]) =>
    createVideoModelPriceRow(model, price),
  );

export const validateVideoModelPriceRows = (
  rows: VideoModelPriceRow[],
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
    for (const [field] of resolutionFields) {
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
): VideoModelPrices => {
  const prices: VideoModelPrices = {};

  for (const row of rows) {
    const model = row.model.trim().toLowerCase();
    if (!model) {
      continue;
    }

    const card: VideoModelPrice = {};
    for (const [field, resolution] of resolutionFields) {
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
  platform === "seedance" ? videoModelPriceRowsToPrices(rows) : undefined;
