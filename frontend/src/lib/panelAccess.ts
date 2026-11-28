// "Only allow access through the domain" (Settings → Domain): the access
// status the API reports per panel domain, and what the switch shows for it.
// Everything here derives from that status; the browser never guesses
// whether it is on the domain.

/** `access` of a GET /api/domains entry. */
export interface PanelAccess {
  /** Dashboard access is restricted to this domain. */
  domainOnly: boolean;
  /** This browser session may turn the restriction on for this domain. */
  canEnable: boolean;
  /** Why canEnable is false. */
  reason?: string;
}

/** Run on the server when the panel domain no longer works. */
export const PANEL_ACCESS_RECOVERY_COMMAND =
  "docker exec docklift-backend docklift panel-access open";

/** The direct dashboard address the restriction closes. */
export function dashboardIPURL(serverIP: string): string {
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(serverIP)) return `http://${serverIP}:8080`;
  if (/^[0-9a-f]*:[0-9a-f:.]*$/i.test(serverIP)) return `http://[${serverIP}]:8080`;
  return "http://SERVER_IP:8080";
}

export interface PanelAccessSwitchState {
  checked: boolean;
  disabled: boolean;
  /** Shown under the switch when it cannot be turned on. */
  hint: string | null;
}

/**
 * The switch of one panel domain. Turning the restriction off is always
 * allowed; turning it on only when the API says this session may.
 */
export function panelAccessSwitchState(
  access: PanelAccess | undefined,
  busy: boolean,
): PanelAccessSwitchState {
  if (!access) {
    return { checked: false, disabled: true, hint: "Add a panel domain with HTTPS first" };
  }
  if (access.domainOnly) return { checked: true, disabled: busy, hint: null };
  if (!access.canEnable) {
    return { checked: false, disabled: true, hint: access.reason || "Not available for this domain" };
  }
  return { checked: false, disabled: busy, hint: null };
}

/**
 * The domain to suggest turning the restriction on for: one this session may
 * enable it for (an HTTPS dashboard domain the page is viewed through), when
 * no domain is restricted yet and the suggestion was not dismissed.
 */
export function panelAccessSuggestion(
  domains: { domain: string; access?: PanelAccess }[],
  isDismissed: (domain: string) => boolean,
): string | null {
  if (domains.some((d) => d.access?.domainOnly)) return null;
  const candidate = domains.find((d) => d.access?.canEnable && !isDismissed(d.domain));
  return candidate ? candidate.domain : null;
}

const DISMISS_KEY = "docklift_panel_access_hint_dismissed";

/** Whether the suggestion for domain was dismissed in this browser. */
export function suggestionDismissed(domain: string): boolean {
  try {
    const raw = localStorage.getItem(DISMISS_KEY);
    return Boolean(raw && (JSON.parse(raw) as string[]).includes(domain));
  } catch {
    return false;
  }
}

/** Remember that the suggestion for domain was dismissed. */
export function dismissSuggestion(domain: string): void {
  try {
    const raw = localStorage.getItem(DISMISS_KEY);
    const list = raw ? (JSON.parse(raw) as string[]) : [];
    if (!list.includes(domain)) list.push(domain);
    localStorage.setItem(DISMISS_KEY, JSON.stringify(list));
  } catch {
    /* storage unavailable: the suggestion comes back on the next visit */
  }
}
