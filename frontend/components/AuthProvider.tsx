// Auth context provider - manages authentication state across the app
"use client";

import { createContext, useContext, useState, useEffect, ReactNode } from "react";
import { useRouter, usePathname } from "next/navigation";
import { API_URL } from "@/lib/utils";
import { Loader2 } from "lucide-react";

interface User {
  id: string;
  name: string;
  email: string;
  role: string;
}

interface AuthContextType {
  user: User | null;
  token: string | null;
  loading: boolean;
  login: (token: string, user: User) => void;
  updateUser: (user: User) => void;
  logout: () => void;
  isAuthenticated: boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}

// Routes that don't require authentication
const publicRoutes = ["/sign-in", "/setup"];

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [initialCheckDone, setInitialCheckDone] = useState(false);
  const router = useRouter();
  const pathname = usePathname();

  const isPublicRoute = publicRoutes.some((route) => pathname.startsWith(route));

  // Initialize auth state from localStorage
  useEffect(() => {
    const initAuth = async () => {
      const storedToken = localStorage.getItem("docklift_token");
      const storedUser = localStorage.getItem("docklift_user");

      if (storedToken && storedUser) {
        // Set token immediately to prevent flicker
        setToken(storedToken);
        try {
          setUser(JSON.parse(storedUser));
        } catch {
          // Invalid stored user
        }

        // Verify token is still valid in background
        try {
          const res = await fetch(`${API_URL}/api/auth/me`, {
            headers: { Authorization: `Bearer ${storedToken}` },
          });

          if (res.ok) {
            const data = await res.json();
            setUser(data.user);
          } else {
            // Token invalid, clear storage
            localStorage.removeItem("docklift_token");
            localStorage.removeItem("docklift_user");
            setToken(null);
            setUser(null);
          }
        } catch (error) {
          console.error("Auth verification failed:", error);
        }
      }

      setLoading(false);
      setInitialCheckDone(true);
    };

    initAuth();
  }, []);

  // Handle redirects after initial check
  useEffect(() => {
    if (!initialCheckDone) return;

    const handleRedirects = async () => {
      // If authenticated and on public route, redirect to dashboard
      if (token && isPublicRoute) {
        router.replace("/");
        return;
      }

      // If not authenticated and not on public route, check setup status
      if (!token && !isPublicRoute) {
        try {
          const res = await fetch(`${API_URL}/api/auth/status`);
          const data = await res.json();

          if (!data.setupComplete) {
            router.replace("/setup");
          } else {
            router.replace("/sign-in");
          }
        } catch (error) {
          router.replace("/sign-in");
        }
      }
    };

    handleRedirects();
  }, [initialCheckDone, token, isPublicRoute, pathname, router]);

  const login = (newToken: string, newUser: User) => {
    localStorage.setItem("docklift_token", newToken);
    localStorage.setItem("docklift_user", JSON.stringify(newUser));
    setToken(newToken);
    setUser(newUser);
  };

  const updateUser = (newUser: User) => {
    localStorage.setItem("docklift_user", JSON.stringify(newUser));
    setUser(newUser);
  };

  const logout = () => {
    localStorage.removeItem("docklift_token");
    localStorage.removeItem("docklift_user");
    setToken(null);
    setUser(null);
    router.push("/sign-in");
  };

  // Show loading only for protected routes during initial check
  if (loading && !isPublicRoute) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-cyan-500" />
      </div>
    );
  }

  return (
    <AuthContext.Provider
      value={{
        user,
        token,
        loading,
        login,
        updateUser,
        logout,
        isAuthenticated: !!token,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

