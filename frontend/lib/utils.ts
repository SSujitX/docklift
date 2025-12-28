// Utility functions - cn() for Tailwind merging, API_URL, fetchAPI helper
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export const API_URL = process.env.NEXT_PUBLIC_API_URL || "";

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
