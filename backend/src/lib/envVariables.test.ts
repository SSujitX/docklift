import { describe, expect, test } from "bun:test";
import { envForService, SHARED_ENV_SERVICE } from "./envVariables.js";

describe("envForService", () => {
  const rows = [
    { key: "SHARED", value: "a", service_name: SHARED_ENV_SERVICE },
    { key: "API_ONLY", value: "b", service_name: "api" },
    { key: "SHARED", value: "override", service_name: "api" },
    { key: "WEB_ONLY", value: "c", service_name: "web" },
  ];

  test("merges shared and service-specific with service winning", () => {
    const api = envForService(rows, "api");
    expect(api.find((r) => r.key === "SHARED")?.value).toBe("override");
    expect(api.find((r) => r.key === "API_ONLY")?.value).toBe("b");
    expect(api.find((r) => r.key === "WEB_ONLY")).toBeUndefined();
  });

  test("returns only shared when service has no overrides", () => {
    const other = envForService(rows, "worker");
    expect(other).toHaveLength(1);
    expect(other[0]?.key).toBe("SHARED");
    expect(other[0]?.value).toBe("a");
  });
});
