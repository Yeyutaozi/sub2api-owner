import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);

describe("video platform group permission", () => {
  it("exposes the media permission for every model-priced video platform", () => {
    expect(groupsViewSource).toMatch(
      /v-if="supportsVideoModelPricingPlatform\(createForm\.platform\)"[\s\S]*?v-model="createForm\.allow_image_generation"[\s\S]*?data-testid="create-seedance-video-enabled"/,
    );
    expect(groupsViewSource).toMatch(
      /v-if="supportsVideoModelPricingPlatform\(editForm\.platform\)"[\s\S]*?v-model="editForm\.allow_image_generation"[\s\S]*?data-testid="edit-seedance-video-enabled"/,
    );
    expect(groupsViewSource).toContain(
      't(videoPricingI18nKey("allowVideoGeneration"))',
    );
    expect(groupsViewSource).toContain('{ value: "seedance", label: "Seedance" }');
    expect(groupsViewSource).toContain('{ value: "ltx", label: "LTX" }');
    expect(groupsViewSource).toContain('{ value: "happyhorse", label: "HappyHorse" }');
  });

  it("offers per-request billing only in the Seedance create and edit forms", () => {
    expect(groupsViewSource).toMatch(
      /v-if="createForm\.platform === 'seedance'"[\s\S]*?data-testid="create-seedance-video-billing-unit"[\s\S]*?<option value="per_request">/,
    );
    expect(groupsViewSource).toMatch(
      /v-if="editForm\.platform === 'seedance'"[\s\S]*?data-testid="edit-seedance-video-billing-unit"[\s\S]*?<option value="per_request">/,
    );
    expect(groupsViewSource).toContain(
      "video_billing_unit: normalizeVideoBillingUnitForPlatform(",
    );
  });

  it("lets each model inherit or override the group billing unit", () => {
    expect(groupsViewSource).toMatch(
      /v-model="row\.billing_unit"[\s\S]*?data-testid="create-video-model-billing-unit"[\s\S]*?<option value="">[\s\S]*?<option value="per_second">[\s\S]*?v-if="supportsPerRequestVideoBilling\(createForm\.platform\)"[\s\S]*?value="per_request"/,
    );
    expect(groupsViewSource).toMatch(
      /v-model="row\.billing_unit"[\s\S]*?data-testid="edit-video-model-billing-unit"[\s\S]*?<option value="">[\s\S]*?<option value="per_second">[\s\S]*?v-if="supportsPerRequestVideoBilling\(editForm\.platform\)"[\s\S]*?value="per_request"/,
    );
    expect(groupsViewSource).toContain(
      "resolveVideoModelBillingUnit(row.billing_unit, createForm.video_billing_unit, createForm.platform)",
    );
    expect(groupsViewSource).toContain(
      "resolveVideoModelBillingUnit(row.billing_unit, editForm.video_billing_unit, editForm.platform)",
    );
    expect(groupsViewSource).toContain("videoModelPricesPayloadForPlatform(");
  });
});
