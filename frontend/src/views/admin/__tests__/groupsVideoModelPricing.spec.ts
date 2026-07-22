import { describe, expect, it } from "vitest";

import {
  createDefaultSeedanceVideoModelPriceRows,
  createVideoModelPriceRow,
  validateVideoModelPriceRows,
  videoModelPriceRowsToPrices,
  videoModelPricesPayloadForPlatform,
  videoModelPricesToRows,
} from "../groupsVideoModelPricing";

describe("Seedance video model pricing form conversion", () => {
  it("starts new Seedance groups with the Pro and Fast model rows", () => {
    expect(createDefaultSeedanceVideoModelPriceRows()).toEqual([
      createVideoModelPriceRow("doubao-seedance-2-0-pro"),
      createVideoModelPriceRow("doubao-seedance-2-0-fast"),
    ]);
  });

  it("serializes model and resolution prices, preserving zero as a free price", () => {
    expect(
      videoModelPriceRowsToPrices([
        {
          model: "  Doubao-Seedance-2-0-Pro ",
          price_480p: 0,
          price_720p: "0.16",
          price_1080p: null,
        },
      ]),
    ).toEqual({
      "doubao-seedance-2-0-pro": {
        "480p": 0,
        "720p": 0.16,
      },
    });
  });

  it("round-trips an API matrix without inventing missing resolution prices", () => {
    const prices = {
      "doubao-seedance-2-0-pro": { "480p": 0, "1080p": 0.2 },
      "doubao-seedance-2-0-fast": { "720p": 0.08 },
    };

    expect(videoModelPricesToRows(prices)).toEqual([
      {
        model: "doubao-seedance-2-0-pro",
        price_480p: 0,
        price_720p: null,
        price_1080p: 0.2,
      },
      {
        model: "doubao-seedance-2-0-fast",
        price_480p: null,
        price_720p: 0.08,
        price_1080p: null,
      },
    ]);
  });

  it("uses an empty object when the matrix is cleared", () => {
    expect(videoModelPriceRowsToPrices([])).toEqual({});
  });

  it("omits the Seedance-only matrix for every other group platform", () => {
    const rows = [createVideoModelPriceRow("pro", { "480p": 0.1 })];

    expect(videoModelPricesPayloadForPlatform("seedance", rows)).toEqual({
      pro: { "480p": 0.1 },
    });
    for (const platform of ["grok", "openai", "gemini", "antigravity", "anthropic"]) {
      expect(videoModelPricesPayloadForPlatform(platform, rows)).toBeUndefined();
    }
  });

  it("rejects blank, duplicate, invalid, and all-empty model rows", () => {
    expect(validateVideoModelPriceRows([createVideoModelPriceRow()])).toEqual({
      code: "modelRequired",
      row: 1,
    });
    expect(
      validateVideoModelPriceRows([
        createVideoModelPriceRow("pro", { "480p": 0.1 }),
        createVideoModelPriceRow(" PRO ", { "720p": 0.2 }),
      ]),
    ).toMatchObject({ code: "duplicateModel", row: 2 });
    expect(
      validateVideoModelPriceRows([
        { model: "pro", price_480p: -1, price_720p: null, price_1080p: null },
      ]),
    ).toMatchObject({ code: "invalidPrice", row: 1 });
    expect(
      validateVideoModelPriceRows([createVideoModelPriceRow("pro")]),
    ).toMatchObject({ code: "priceRequired", row: 1 });
  });
});
