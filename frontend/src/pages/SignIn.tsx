// Login page

import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Container, Eye, EyeOff, Loader2, AlertCircle } from "lucide-react";
import { API_URL } from "@/lib/utils";
import { useAuth } from "@/components/AuthProvider";

export default function LoginPage() {
  const navigate = useNavigate();
  const { login } = useAuth();
  const [formData, setFormData] = useState({
    email: "",
    password: "",
  });
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const res = await fetch(`${API_URL}/api/auth/login`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(formData),
      });

      const data = await res.json();

      if (!res.ok) {
        throw new Error(data.error || "Login failed");
      }

      // Use AuthProvider's login function to update state and redirect
      login(data.token, data.user);
      
      // Redirect to dashboard
      navigate("/");
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4 pt-10 pb-[12vh]">
      <div className="w-full max-w-[360px]">
        <div className="mb-6 flex items-center justify-center gap-2">
          <span className="flex size-7 items-center justify-center rounded-md bg-brand">
            <Container className="size-4 text-brand-foreground" />
          </span>
          <span className="text-[15px] font-semibold text-foreground">Docklift</span>
        </div>

        <Card className="p-5 sm:p-6">
          <div className="mb-5">
            <h1 className="text-xl font-semibold leading-7 tracking-[-0.01em]">Sign in</h1>
            <p className="mt-0.5 text-[13.5px] leading-5 text-muted-foreground">
              Sign in to access your dashboard.
            </p>
          </div>

          {error && (
            <div className="mb-4 flex items-start gap-2 rounded-md border border-danger/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5 text-foreground">
              <AlertCircle className="mt-0.5 size-4 shrink-0 text-danger" />
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="mb-1.5 block text-[13px] font-medium">Email</label>
              <Input
                type="email"
                value={formData.email}
                onChange={(e) => setFormData({ ...formData, email: e.target.value })}
                placeholder="admin@example.com"
                required
              />
            </div>

            <div>
              <label className="mb-1.5 block text-[13px] font-medium">Password</label>
              <div className="relative">
                <Input
                  type={showPassword ? "text" : "password"}
                  value={formData.password}
                  onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                  placeholder="••••••••"
                  required
                  className="pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-1 top-1/2 flex size-7 -translate-y-1/2 items-center justify-center rounded-sm text-subtle transition-colors duration-150 hover:text-foreground pointer-coarse:size-9"
                >
                  {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                </button>
              </div>
            </div>

            <Button
              type="submit"
              size="lg"
              disabled={loading || !formData.email || !formData.password}
              className="w-full"
            >
              {loading && <Loader2 className="animate-spin" />}
              Sign in
            </Button>
          </form>
        </Card>
      </div>
    </div>
  );
}
