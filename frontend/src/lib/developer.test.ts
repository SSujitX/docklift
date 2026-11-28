import { describe, expect, test } from "bun:test";
import {
  claudeCodeCommand,
  countEndpoints,
  describeTime,
  filterEndpoints,
  genericMcpConfig,
  mcpUrl,
  parseOpenApi,
} from "./developer";

const doc = {
  openapi: "3.1.0",
  tags: [
    { name: "Projects", description: "Apps" },
    { name: "System", description: "Host" },
    { name: "Empty" },
  ],
  paths: {
    "/api/projects/{id}/env": {
      post: {
        tags: ["Projects"],
        summary: "Create an environment variable",
        parameters: [{ name: "id", in: "path", required: true, schema: { type: "string" } }],
        requestBody: {
          content: {
            "application/json": {
              schema: {
                type: "object",
                properties: { key: { type: "string" }, service_name: { type: ["string", "null"] } },
                required: ["key"],
              },
            },
          },
        },
        "x-docklift-read-scope": false,
      },
      get: { tags: ["Projects"], summary: "List environment variables", "x-docklift-read-scope": true },
    },
    "/api/system/reboot": {
      post: { tags: ["System"], summary: "Reboot the host", "x-docklift-step-up": true },
    },
  },
};

describe("parseOpenApi", () => {
  test("groups by tag in document order and reads body fields", () => {
    const groups = parseOpenApi(doc);
    expect(groups.map((g) => g.tag)).toEqual(["Projects", "System"]);
    const [list, create] = groups[0].endpoints;
    expect(list.method).toBe("GET");
    expect(list.readScope).toBe(true);
    expect(create.body).toEqual([
      { name: "key", type: "string", required: true, description: undefined },
      { name: "service_name", type: "string | null", required: false, description: undefined },
    ]);
    expect(create.params[0]).toMatchObject({ name: "id", in: "path", required: true });
    expect(groups[1].endpoints[0].stepUp).toBe(true);
  });

  test("survives junk", () => {
    expect(parseOpenApi(null)).toEqual([]);
    expect(parseOpenApi({ paths: { "/x": { get: "nope" } } })).toEqual([]);
  });
});

describe("filterEndpoints", () => {
  const groups = parseOpenApi(doc);
  test("matches every word across method, path and summary", () => {
    expect(countEndpoints(filterEndpoints(groups, "post env"))).toBe(1);
    expect(countEndpoints(filterEndpoints(groups, "reboot"))).toBe(1);
    expect(countEndpoints(filterEndpoints(groups, ""))).toBe(3);
    expect(filterEndpoints(groups, "nothing-matches")).toEqual([]);
  });
});

describe("snippets", () => {
  test("mcp url and client configs", () => {
    const url = mcpUrl("https://panel.example.com/");
    expect(url).toBe("https://panel.example.com/mcp");
    expect(mcpUrl("http://x", "https://api.example.com")).toBe("https://api.example.com/mcp");
    expect(claudeCodeCommand(url, "dl_abc")).toBe(
      'claude mcp add --transport http docklift https://panel.example.com/mcp --header "Authorization: Bearer dl_abc"',
    );
    const cfg = JSON.parse(genericMcpConfig(url, "dl_abc"));
    expect(cfg.mcpServers.docklift).toEqual({ type: "http", url, headers: { Authorization: "Bearer dl_abc" } });
  });
});

describe("describeTime", () => {
  const now = Date.parse("2026-11-28T12:00:00.000Z");
  test("past, future and empty", () => {
    expect(describeTime(null, now)).toBe("Never");
    expect(describeTime("2026-11-28T11:58:00.000Z", now)).toBe("2 minutes ago");
    expect(describeTime("2026-12-28T12:00:00.000Z", now)).toBe("in 1 month");
    expect(describeTime("2026-11-28T11:59:59.000Z", now)).toBe("just now");
  });
});
