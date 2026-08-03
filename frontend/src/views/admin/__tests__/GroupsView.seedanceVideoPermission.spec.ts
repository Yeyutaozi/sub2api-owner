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
});
