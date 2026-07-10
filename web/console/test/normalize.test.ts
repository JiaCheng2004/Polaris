import { describe, expect, it } from "vitest";
import { normalizeBudget, normalizeMCPBinding, normalizePolicy } from "../src/api/normalize";

// The gateway serializes these control-plane resources as raw Go structs
// (PascalCase). The console must read them regardless of casing.
describe("control-plane response normalization", () => {
  it("reads PascalCase policy structs", () => {
    const p = normalizePolicy({
      ID: "pol_key_1",
      ProjectID: "proj_1",
      Name: "reads",
      AllowedModels: ["openai/gpt-4o"],
      AllowedMCP: ["mcp_1"],
      CreatedAt: "2026-07-01T00:00:00Z",
    });
    expect(p.id).toBe("pol_key_1");
    expect(p.allowed_models).toEqual(["openai/gpt-4o"]);
    expect(p.allowed_mcp).toEqual(["mcp_1"]);
  });

  it("reads snake_case too (forward-compatible if the API is fixed)", () => {
    const p = normalizePolicy({ id: "pol_2", project_id: "proj_2", name: "x", allowed_models: [] });
    expect(p.id).toBe("pol_2");
    expect(p.project_id).toBe("proj_2");
  });

  it("normalizes budget numeric + enum fields", () => {
    const b = normalizeBudget({ ID: "bud_1", Mode: "hard", LimitUSD: 25, Window: "monthly" });
    expect(b.mode).toBe("hard");
    expect(b.limit_usd).toBe(25);
    expect(b.window).toBe("monthly");
  });

  it("defaults missing enums safely", () => {
    const b = normalizeBudget({ ID: "bud_2" });
    expect(b.mode).toBe("soft");
    expect(b.window).toBe("monthly");
    const m = normalizeMCPBinding({ ID: "mcp_2" });
    expect(m.kind).toBe("upstream_proxy");
  });
});
