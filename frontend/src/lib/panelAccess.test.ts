import { describe, expect, test } from "bun:test";
import {
  dashboardIPURL,
  panelAccessSuggestion,
  panelAccessSwitchState,
  type PanelAccess,
} from "./panelAccess";

const open = "Open the dashboard at https://panel.example.com first, then turn this on";

describe("panelAccessSwitchState", () => {
  test("disabled with a reason when there is no panel domain", () => {
    expect(panelAccessSwitchState(undefined, false)).toEqual({
      checked: false,
      disabled: true,
      hint: "Add a panel domain with HTTPS first",
    });
  });

  test("disabled with the API reason when this session may not enable it", () => {
    const access: PanelAccess = { domainOnly: false, canEnable: false, reason: open };
    expect(panelAccessSwitchState(access, false)).toEqual({ checked: false, disabled: true, hint: open });
  });

  test("enabled when the API allows it, except while busy", () => {
    const access: PanelAccess = { domainOnly: false, canEnable: true };
    expect(panelAccessSwitchState(access, false)).toEqual({ checked: false, disabled: false, hint: null });
    expect(panelAccessSwitchState(access, true).disabled).toBe(true);
  });

  test("turning it off is always allowed", () => {
    const access: PanelAccess = { domainOnly: true, canEnable: false, reason: open };
    expect(panelAccessSwitchState(access, false)).toEqual({ checked: true, disabled: false, hint: null });
  });
});

describe("panelAccessSuggestion", () => {
  const none = () => false;
  test("suggests the domain the page is viewed through", () => {
    expect(
      panelAccessSuggestion(
        [
          { domain: "other.example.com", access: { domainOnly: false, canEnable: false, reason: "x" } },
          { domain: "panel.example.com", access: { domainOnly: false, canEnable: true } },
        ],
        none,
      ),
    ).toBe("panel.example.com");
  });

  test("nothing when viewed by IP, already restricted, or dismissed", () => {
    expect(panelAccessSuggestion([{ domain: "panel.example.com", access: { domainOnly: false, canEnable: false, reason: open } }], none)).toBeNull();
    expect(
      panelAccessSuggestion(
        [
          { domain: "a.example.com", access: { domainOnly: true, canEnable: false } },
          { domain: "panel.example.com", access: { domainOnly: false, canEnable: true } },
        ],
        none,
      ),
    ).toBeNull();
    expect(
      panelAccessSuggestion([{ domain: "panel.example.com", access: { domainOnly: false, canEnable: true } }], (d) => d === "panel.example.com"),
    ).toBeNull();
    expect(panelAccessSuggestion([{ domain: "panel.example.com" }], none)).toBeNull();
  });
});

describe("dashboardIPURL", () => {
  test("formats IPv4, IPv6 and unknown addresses", () => {
    expect(dashboardIPURL("203.0.113.9")).toBe("http://203.0.113.9:8080");
    expect(dashboardIPURL("2001:db8::1")).toBe("http://[2001:db8::1]:8080");
    expect(dashboardIPURL("...")).toBe("http://SERVER_IP:8080");
    expect(dashboardIPURL("")).toBe("http://SERVER_IP:8080");
  });
});
