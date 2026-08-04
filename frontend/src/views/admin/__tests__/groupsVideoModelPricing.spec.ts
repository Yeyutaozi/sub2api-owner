import { describe, expect, it } from "vitest";

import {
  createDefaultVideoModelPriceRows,
  createVideoModelPriceRow,
  supportsVideoModelPricingPlatform,
  supportedResolutionsForVideoModel,
  validateVideoModelPriceRows,
  videoModelPricePlaceholder,
  videoModelPriceRowsToPrices,
  videoModelPricesPayloadForPlatform,
  videoModelPricesToRows,
} from "../groupsVideoModelPricing";

describe("video model pricing form conversion", () => {
  it("uses an FFLink video platform gate", () => {
    expect(supportsVideoModelPricingPlatform("seedance")).toBe(true);
    expect(supportsVideoModelPricingPlatform("ltx")).toBe(true);
    expect(supportsVideoModelPricingPlatform("happyhorse")).toBe(true);
    for (const platform of ["grok", "openai", "gemini", "antigravity", "anthropic"]) {
      expect(supportsVideoModelPricingPlatform(platform)).toBe(false);
    }
  });

  it("starts new groups with platform-specific model IDs", () => {
    expect(createDefaultVideoModelPriceRows()).toEqual([
      createVideoModelPriceRow("seedance-2.0"),
      createVideoModelPriceRow("seedance-2.0-fast"),
      createVideoModelPriceRow("seedance-2.0-mini"),
      createVideoModelPriceRow("sd2-mx933-720-1s"),
      createVideoModelPriceRow("sd2-mx933-720-fast-1s"),
    ]);
    expect(createDefaultVideoModelPriceRows("ltx")).toEqual([
      createVideoModelPriceRow("ltx-2.3-pro"),
      createVideoModelPriceRow("ltx-2.3-fast"),
    ]);
    expect(createDefaultVideoModelPriceRows("happyhorse")).toEqual([
      createVideoModelPriceRow("happy-horse-1.1"),
    ]);
    expect(videoModelPricePlaceholder("seedance")).toBe("seedance-2.0");
    expect(videoModelPricePlaceholder("ltx")).toBe("ltx-2.3-pro");
    expect(videoModelPricePlaceholder("happyhorse")).toBe("happy-horse-1.1");
  });

  it("matches each platform model's supported pricing resolutions", () => {
    expect(supportedResolutionsForVideoModel("seedance", "seedance-2.0-mini")).toEqual([
      "480p",
      "720p",
    ]);
    expect(supportedResolutionsForVideoModel("seedance", "sd2-mx933-720-1s")).toEqual([
      "480p",
      "720p",
    ]);
    expect(supportedResolutionsForVideoModel("seedance", "sd2-mx933-720-fast-1s")).toEqual([
      "480p",
      "720p",
    ]);
    expect(supportedResolutionsForVideoModel("ltx", "ltx-2.3-fast")).toEqual([
      "1080p",
      "1440p",
      "2160p",
    ]);
    expect(supportedResolutionsForVideoModel("happyhorse", "happy-horse-1.1")).toEqual([
      "720p",
      "1080p",
    ]);
  });

  it("serializes model and resolution prices, preserving zero as a free price", () => {
    expect(
      videoModelPriceRowsToPrices([
        {
          model: "  Seedance-2.0 ",
          price_480p: 0,
          price_720p: "0.16",
          price_1080p: null,
          price_1440p: null,
          price_2160p: null,
        },
      ]),
    ).toEqual({
      "seedance-2.0": {
        "480p": 0,
        "720p": 0.16,
      },
    });
  });

  it("round-trips an API matrix without inventing missing resolution prices", () => {
    const prices = {
      "seedance-2.0": { "480p": 0, "1080p": 0.2 },
      "seedance-2.0-fast": { "720p": 0.08 },
    };

    expect(videoModelPricesToRows(prices)).toEqual([
      {
        model: "seedance-2.0",
        price_480p: 0,
        price_720p: null,
        price_1080p: 0.2,
        price_1440p: null,
        price_2160p: null,
      },
      {
        model: "seedance-2.0-fast",
        price_480p: null,
        price_720p: 0.08,
        price_1080p: null,
        price_1440p: null,
        price_2160p: null,
      },
    ]);
  });

  it("preserves existing custom aliases when editing a group", () => {
    const legacyPrices = {
      "doubao-seedance-2-0-pro": { "720p": 0.16 },
    };

    expect(
      videoModelPriceRowsToPrices(videoModelPricesToRows(legacyPrices)),
    ).toEqual(legacyPrices);
  });

  it("uses an empty object when the matrix is cleared", () => {
    expect(videoModelPriceRowsToPrices([])).toEqual({});
  });

  it("drops prices for resolutions unsupported by the selected model", () => {
    expect(
      videoModelPriceRowsToPrices([
        createVideoModelPriceRow("seedance-2.0-mini", {
          "720p": 0.04,
          "1080p": 0.1,
        }),
      ], "seedance"),
    ).toEqual({
      "seedance-2.0-mini": { "720p": 0.04 },
    });

    expect(
      videoModelPriceRowsToPrices([
        createVideoModelPriceRow("sd2-mx933-720-1s", {
          "480p": 0.03,
          "720p": 0.05,
          "1080p": 0.08,
        }),
      ], "seedance"),
    ).toEqual({
      "sd2-mx933-720-1s": { "480p": 0.03, "720p": 0.05 },
    });

    expect(
      videoModelPriceRowsToPrices([
        createVideoModelPriceRow("ltx-2.3-pro", {
          "720p": 0.1,
          "1440p": 0.2,
          "2160p": 0.3,
        }),
      ], "ltx"),
    ).toEqual({
      "ltx-2.3-pro": { "1440p": 0.2, "2160p": 0.3 },
    });
  });

  it("omits the video matrix for every non-FFLink group platform", () => {
    const rows = [createVideoModelPriceRow("pro", { "480p": 0.1 })];

    expect(videoModelPricesPayloadForPlatform("seedance", rows)).toEqual({
      pro: { "480p": 0.1 },
    });
    expect(videoModelPricesPayloadForPlatform("ltx", [
      createVideoModelPriceRow("ltx-2.3-pro", { "1440p": 0.2 }),
    ])).toEqual({ "ltx-2.3-pro": { "1440p": 0.2 } });
    expect(videoModelPricesPayloadForPlatform("happyhorse", [
      createVideoModelPriceRow("happy-horse-1.1", { "1080p": 0.3 }),
    ])).toEqual({ "happy-horse-1.1": { "1080p": 0.3 } });
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
