// Utility functions - cn() for Tailwind merging, API_URL, fetchAPI helper
import { clsx, type ClassValue } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";

/**
 * tailwind-merge does not know the v3 `bg-gradient-to-*` utilities, so it filed them under the
 * background-colour group: a later `bg-<colour>` replaced the gradient and a gradient replaced an
 * earlier `bg-<colour>`. Components (e.g. `<Button className="bg-brand">`) rely on that. The v4
 * `bg-linear-to-*` utilities are a separate group, so register them under the same group to keep
 * the merge result unchanged.
 */
const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      "bg-color": [{ "bg-linear-to": ["t", "tr", "r", "br", "b", "bl", "l", "tl"] }],
    },
  },
});

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export const API_URL = import.meta.env.VITE_API_URL || "";

export async function fetchAPI(endpoint: string, options?: RequestInit) {
  const res = await fetch(`${API_URL}${endpoint}`, {
    ...options,
    headers: {
      ...options?.headers,
    },
  });

  if (!res.ok) {
    throw new Error(`API Error: ${res.status}`);
  }

  return res.json();
}

/**
 * Copy text to clipboard with fallback for non-secure contexts (HTTP).
 * navigator.clipboard only works on HTTPS or localhost, so we need a fallback
 * using the older document.execCommand('copy') method for HTTP connections.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  // Try modern clipboard API first (works on HTTPS and localhost)
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall through to fallback
    }
  }

  // Fallback for HTTP contexts using hidden textarea
  try {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.left = '-9999px';
    textarea.style.top = '-9999px';
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();
    
    // Note: execCommand is deprecated but is the ONLY fallback for HTTP contexts.
    // navigator.clipboard is blocked by browsers on non-secure origins.
    // This is intentional and browsers will continue to support execCommand.
    // eslint-disable-next-line @typescript-eslint/no-deprecated
    const success = document.execCommand('copy');
    document.body.removeChild(textarea);
    return success;
  } catch {
    return false;
  }
}
