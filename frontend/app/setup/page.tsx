// Setup page - first-time registration for root admin
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Container, Eye, EyeOff, Loader2, Check, AlertCircle } from "lucide-react";
import { API_URL } from "@/lib/utils";

export default function SetupPage() {
  const router = useRouter();
  const [formData, setFormData] = useState({
    name: "",
    email: "",
    password: "",
    confirmPassword: "",
  });
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const validatePassword = (password: string) => {
    return password.length >= 8;
  };

  const isPasswordValid = validatePassword(formData.password);
  const passwordsMatch = formData.password === formData.confirmPassword && formData.confirmPassword.length > 0;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");

    if (!isPasswordValid) {
      setError("Password must be at least 8 characters");
      return;
    }

    if (!passwordsMatch) {
      setError("Passwords do not match");
      return;
    }

    setLoading(true);
    try {
      const res = await fetch(`${API_URL}/api/auth/register`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: formData.name,
          email: formData.email,
          password: formData.password,
        }),
      });

      const data = await res.json();

      if (!res.ok) {
        throw new Error(data.error || "Registration failed");
      }

      // Store token
      localStorage.setItem("docklift_token", data.token);
      localStorage.setItem("docklift_user", JSON.stringify(data.user));

      // Redirect to dashboard
      router.push("/");
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-background via-background to-cyan-950/20 p-4">
      <div className="w-full max-w-md">
        {/* Logo */}
        <div className="flex flex-col items-center mb-8">
          <div className="relative mb-4">
            <div className="absolute inset-0 bg-gradient-to-br from-cyan-500 to-blue-600 rounded-2xl blur-xl opacity-50" />
            <div className="relative bg-gradient-to-br from-cyan-500 to-blue-600 p-4 rounded-2xl shadow-2xl">
              <Container className="h-10 w-10 text-white" strokeWidth={2.5} />
            </div>
          </div>
          <h1 className="text-4xl font-black tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-foreground to-foreground/70">
            Docklift
          </h1>
        </div>

        <Card className="p-8 border-border/50 shadow-2xl bg-card/80 backdrop-blur-sm">
          <div className="mb-6">
            <h2 className="text-2xl font-bold tracking-tight">Create an account</h2>
            <p className="text-sm text-cyan-500 mt-1 font-medium">
              This user will be the root user (full admin access).
            </p>
          </div>

          {error && (
            <div className="mb-4 p-3 rounded-lg bg-red-500/10 border border-red-500/20 flex items-center gap-2 text-red-500 text-sm">
              <AlertCircle className="h-4 w-4 shrink-0" />
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="text-sm font-medium mb-1.5 block">
                Name <span className="text-red-500">*</span>
              </label>
              <Input
                type="text"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="Your name"
                required
                className="h-11"
              />
            </div>

            <div>
              <label className="text-sm font-medium mb-1.5 block">
                Email <span className="text-red-500">*</span>
              </label>
              <Input
                type="email"
                value={formData.email}
                onChange={(e) => setFormData({ ...formData, email: e.target.value })}
                placeholder="admin@example.com"
                required
                className="h-11"
              />
            </div>

            <div>
              <label className="text-sm font-medium mb-1.5 block">
                Password <span className="text-red-500">*</span>
              </label>
              <div className="relative">
                <Input
                  type={showPassword ? "text" : "password"}
                  value={formData.password}
                  onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                  placeholder="••••••••"
                  required
                  className="h-11 pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              {formData.password.length > 0 && formData.password.length < 8 && (
                <p className="text-xs text-muted-foreground mt-1">Password must be at least 8 characters</p>
              )}
            </div>

            <div>
              <label className="text-sm font-medium mb-1.5 block">
                Confirm Password <span className="text-red-500">*</span>
              </label>
              <div className="relative">
                <Input
                  type={showConfirmPassword ? "text" : "password"}
                  value={formData.confirmPassword}
                  onChange={(e) => setFormData({ ...formData, confirmPassword: e.target.value })}
                  placeholder="••••••••"
                  required
                  className="h-11 pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  {showConfirmPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              {formData.confirmPassword && !passwordsMatch && (
                <p className="text-xs text-red-500 mt-1">Passwords do not match</p>
              )}
            </div>

            <Button
              type="submit"
              disabled={loading || !isPasswordValid || !passwordsMatch || !formData.name || !formData.email}
              className="w-full h-11 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white font-bold"
            >
              {loading ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
              Register
            </Button>
          </form>

          <p className="text-center text-sm text-muted-foreground mt-6">
            Already registered?{" "}
            <a href="/sign-in" className="text-cyan-500 hover:underline font-medium">
              Sign In
            </a>
          </p>
        </Card>
      </div>
    </div>
  );
}

function PasswordCheck({ label, valid }: { label: string; valid: boolean }) {
  return (
    <div className={`flex items-center gap-1.5 ${valid ? "text-emerald-500" : "text-muted-foreground"}`}>
      <Check className={`h-3 w-3 ${valid ? "opacity-100" : "opacity-30"}`} />
      <span>{label}</span>
    </div>
  );
}
