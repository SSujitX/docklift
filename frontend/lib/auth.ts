// Utility hook for making authenticated API calls
"use client";

import { API_URL } from "@/lib/utils";

// Get auth headers for API calls
export function getAuthHeaders(): HeadersInit {
  if (typeof window === "undefined") return {};
  
  const token = localStorage.getItem("docklift_token");
  return token ? { Authorization: `Bearer ${token}` } : {};
}

// Authenticated fetch wrapper
export async function authFetch(url: string, options: RequestInit = {}): Promise<Response> {
  const headers = {
    ...getAuthHeaders(),
    ...options.headers,
  };

  return fetch(url, {
    ...options,
    headers,
  });
}

// Typed fetch that handles auth and returns JSON
export async function fetchWithAuth<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const url = endpoint.startsWith("http") ? endpoint : `${API_URL}${endpoint}`;
  
  const response = await authFetch(url, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options.headers,
    },
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: "Request failed" }));
    throw new Error(error.error || `HTTP ${response.status}`);
  }

  return response.json();
}
