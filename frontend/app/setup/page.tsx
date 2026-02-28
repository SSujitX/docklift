// Setup page - first-time registration for root admin
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/components/AuthProvider";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Container, Eye, EyeOff, Loader2, Check, AlertCircle, Upload, RotateCcw, FileUp, ArrowLeft } from "lucide-react";
import { API_URL } from "@/lib/utils";

export default function SetupPage() {
  const router = useRouter();
  const auth = useAuth();
  const [mode, setMode] = useState<'register' | 'restore'>('register');
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

  // Restore state
  const [restoreProgress, setRestoreProgress] = useState<string[]>([]);
  const [restoring, setRestoring] = useState(false);
  const [restoreComplete, setRestoreComplete] = useState(false);
  const [isDragging, setIsDragging] = useState(false);

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

      // Store token and user in global auth state
      auth.login(data.token, data.user);

      // Redirect to dashboard
      router.push("/");
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const processRestoreFile = async (file: File) => {
    if (!file.name.endsWith('.zip')) {
      setError("Please select a .zip backup file");
      return;
    }

    setError("");
    setRestoring(true);
    setRestoreProgress([]);

    try {
      const formData = new FormData();
      formData.append('backup', file);

      // Fetch one-time setup token for unauthenticated restore (fresh install)
      let headers: HeadersInit = {};
      try {
        const tokenRes = await fetch(`${API_URL}/api/auth/setup-token`);
        if (tokenRes.ok) {
          const tokenData = await tokenRes.json();
          if (tokenData.setupToken) {
            headers['x-setup-token'] = tokenData.setupToken;
          }
        }
      } catch { /* If token fetch fails, try without — backend will decide */ }

      const res = await fetch(`${API_URL}/api/backup/restore-upload`, {
        method: 'POST',
        headers,
        body: formData,
      });

      const reader = res.body?.getReader();
      const decoder = new TextDecoder();

      if (reader) {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          const text = decoder.decode(value);
          const lines = text.split('\n').filter(line => line.trim());
          setRestoreProgress(prev => [...prev, ...lines]);
        }
      }

      setRestoreComplete(true);
    } catch (err: any) {
      setError(err.message || "Restore failed");
    } finally {
      setRestoring(false);
    }
  };

  const handleRestoreFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    await processRestoreFile(file);
    e.target.value = '';
  };

  // Drag and drop handlers
  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
  };

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);

    const file = e.dataTransfer.files?.[0];
    if (file) {
      await processRestoreFile(file);
    }
  };

  // Show restore progress/complete view
  if (restoring || restoreComplete) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-background via-background to-amber-950/20 p-4">
        <div className="w-full max-w-2xl">
          <Card className="p-8 border-amber-500/20 shadow-2xl bg-card/80 backdrop-blur-sm">
            <div className="flex items-center gap-3 mb-6">
              <div className="p-3 rounded-xl bg-amber-500/10">
                <RotateCcw className={`h-6 w-6 text-amber-500 ${restoring ? 'animate-spin' : ''}`} />
              </div>
              <div>
                <h2 className="text-2xl font-bold">
                  {restoreComplete ? 'Restore Complete' : 'Restoring Backup...'}
                </h2>
                <p className="text-sm text-muted-foreground">
                  {restoreComplete ? 'Your Docklift instance has been restored' : 'Please wait while your backup is being restored'}
                </p>
              </div>
            </div>

            <div className="bg-black/90 rounded-lg p-4 font-mono text-xs text-green-400 max-h-[400px] overflow-y-auto mb-6">
              {restoreProgress.map((line, i) => (
                <div key={i} className="whitespace-pre-wrap">{line}</div>
              ))}
              {restoring && <span className="animate-pulse">▌</span>}
            </div>

            {restoreComplete && (
              <div className="space-y-4">
                <div className="p-4 rounded-lg bg-emerald-500/10 border border-emerald-500/20">
                  <p className="text-emerald-500 font-medium">Restore successful!</p>
                  <p className="text-sm text-muted-foreground mt-1">
                    You can now sign in with your existing credentials from the backup.
                  </p>
                </div>
                <Button
                  onClick={() => router.push('/sign-in')}
                  className="w-full h-11 bg-gradient-to-r from-emerald-500 to-green-600 hover:from-emerald-600 hover:to-green-700 text-white font-bold"
                >
                  Go to Sign In
                </Button>
              </div>
            )}
          </Card>
        </div>
      </div>
    );
  }

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

        {mode === 'register' ? (
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

            <div className="relative my-6">
              <div className="absolute inset-0 flex items-center">
                <div className="w-full border-t border-border/50"></div>
              </div>
              <div className="relative flex justify-center text-xs uppercase">
                <span className="bg-card px-2 text-muted-foreground">Or</span>
              </div>
            </div>

            <Button
              type="button"
              variant="outline"
              onClick={() => setMode('restore')}
              className="w-full h-11 border-amber-500/30 text-amber-500 hover:bg-amber-500/10"
            >
              <RotateCcw className="h-4 w-4 mr-2" />
              Restore from Backup
            </Button>

            <p className="text-center text-sm text-muted-foreground mt-6">
              Already registered?{" "}
              <a href="/sign-in" className="text-cyan-500 hover:underline font-medium">
                Sign In
              </a>
            </p>
          </Card>
        ) : (
          <Card className="p-8 border-amber-500/20 shadow-2xl bg-card/80 backdrop-blur-sm">
            <button
              onClick={() => setMode('register')}
              className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground mb-4 transition-colors"
            >
              <ArrowLeft className="h-4 w-4" />
              Back to registration
            </button>

            <div className="mb-6">
              <h2 className="text-2xl font-bold tracking-tight">Restore from Backup</h2>
              <p className="text-sm text-amber-500 mt-1 font-medium">
                Restore your Docklift instance from a backup file.
              </p>
            </div>

            {error && (
              <div className="mb-4 p-3 rounded-lg bg-red-500/10 border border-red-500/20 flex items-center gap-2 text-red-500 text-sm">
                <AlertCircle className="h-4 w-4 shrink-0" />
                {error}
              </div>
            )}

            <div
              className={`p-6 rounded-lg border-2 border-dashed transition-all ${
                isDragging
                  ? 'border-amber-500 bg-amber-500/10 scale-[1.02]'
                  : 'border-amber-500/30 bg-amber-500/5'
              }`}
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
            >
              <div className="flex flex-col items-center gap-4 text-center">
                <div className={`p-4 rounded-full transition-all ${isDragging ? 'bg-amber-500/20 scale-110' : 'bg-amber-500/10'}`}>
                  <FileUp className={`h-8 w-8 text-amber-500 ${isDragging ? 'animate-bounce' : ''}`} />
                </div>
                <div>
                  <p className="font-medium">
                    {isDragging ? 'Drop your backup file here' : 'Drag & drop or select a backup file'}
                  </p>
                  <p className="text-sm text-muted-foreground mt-1">
                    {isDragging ? 'Release to upload' : 'Select a .zip backup file from your computer'}
                  </p>
                </div>
                <label className="cursor-pointer">
                  <input
                    type="file"
                    accept=".zip"
                    onChange={handleRestoreFile}
                    className="hidden"
                    disabled={restoring}
                  />
                  <Button
                    type="button"
                    className="bg-amber-600 hover:bg-amber-700"
                    disabled={restoring}
                    asChild
                  >
                    <span>
                      {restoring ? (
                        <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      ) : (
                        <Upload className="h-4 w-4 mr-2" />
                      )}
                      Select Backup File
                    </span>
                  </Button>
                </label>
              </div>
            </div>

            <div className="mt-4 p-3 rounded-lg bg-secondary/30 border border-border/50 text-sm">
              <p className="font-medium mb-2">Backup will restore:</p>
              <ul className="list-disc list-inside text-muted-foreground space-y-1 text-xs">
                <li>All user accounts and credentials</li>
                <li>Projects and deployments</li>
                <li>Environment variables</li>
                <li>Nginx configurations</li>
                <li>GitHub App settings</li>
              </ul>
            </div>

            <div className="mt-3 p-3 rounded-lg bg-cyan-500/10 border border-cyan-500/20 text-sm">
              <p className="font-medium text-cyan-500 mb-2">After restore:</p>
              <ul className="list-disc list-inside text-muted-foreground space-y-1 text-xs">
                <li>Sign in with your credentials from the backup</li>
                <li>Redeploy each project (containers need rebuilding)</li>
                <li>Update DNS if server IP changed</li>
                <li>GitHub App works automatically if using domain-based webhook URL</li>
              </ul>
            </div>
          </Card>
        )}
      </div>
    </div>
  );
}
