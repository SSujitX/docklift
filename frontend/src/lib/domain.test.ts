import { describe, expect, test } from "bun:test";
import {
  displayHostnameFromInput,
  dnsRecordHint,
  normalizeDomainInput,
} from "./domain";

describe("normalizeDomainInput", () => {
  test("strips https and trailing slash only", () => {
    expect(normalizeDomainInput("https://docklift.dev/").value).toBe("docklift.dev");
    expect(normalizeDomainInput("https://chandralight.co.uk/").value).toBe(
      "chandralight.co.uk",
    );
  });

  test("keeps www and deeper subdomains", () => {
    expect(normalizeDomainInput("https://www.chandralight.co.uk").value).toBe(
      "www.chandralight.co.uk",
    );
    expect(normalizeDomainInput("https://api.v2.app.example.com/").value).toBe(
      "api.v2.app.example.com",
    );
  });

  test("strips path but not host labels", () => {
    expect(normalizeDomainInput("https://app.example.com/dashboard?x=1").value).toBe(
      "app.example.com",
    );
  });
});

describe("displayHostnameFromInput", () => {
  test("rewrites pasted URLs to bare host", () => {
    expect(displayHostnameFromInput("https://docklift.dev/")).toBe("docklift.dev");
    expect(displayHostnameFromInput("https://www.chandralight.co.uk/")).toBe(
      "www.chandralight.co.uk",
    );
  });

  test("leaves plain host typing alone", () => {
    expect(displayHostnameFromInput("app.ex")).toBe("app.ex");
    expect(displayHostnameFromInput("chandralight.co.uk")).toBe("chandralight.co.uk");
  });
});

describe("dnsRecordHint", () => {
  test("co.uk keeps registrable domain", () => {
    expect(dnsRecordHint("chandralight.co.uk").name).toBe("@");
    expect(dnsRecordHint("www.chandralight.co.uk").name).toBe("www");
    expect(dnsRecordHint("api.v2.chandralight.co.uk").name).toBe("api.v2");
  });
});
