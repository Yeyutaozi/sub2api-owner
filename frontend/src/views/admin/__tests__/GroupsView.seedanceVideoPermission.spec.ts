import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);

describe("Seedance group video permission", () => {
  it("exposes the existing media permission as a Seedance-only video switch", () => {
    expect(groupsViewSource).toMatch(
      /v-if="createForm\.platform === 'seedance'"[\s\S]*?v-model="createForm\.allow_image_generation"[\s\S]*?data-testid="create-seedance-video-enabled"/,
    );
    expect(groupsViewSource).toMatch(
      /v-if="editForm\.platform === 'seedance'"[\s\S]*?v-model="editForm\.allow_image_generation"[\s\S]*?data-testid="edit-seedance-video-enabled"/,
    );
    expect(groupsViewSource).toContain(
      't(videoPricingI18nKey("allowVideoGeneration"))',
    );
  });
});
