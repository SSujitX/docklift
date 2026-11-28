// Setup page - first-time registration for root admin

import { useState } from "react";
import { useNavigate, Link } from "react-router-dom";
import { useAuth } from "@/components/AuthProvider";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Container, Eye, EyeOff, Loader2, Check, AlertCircle, Upload, RotateCcw, FileUp, ArrowLeft } from "lucide-react";
import { API_URL } from "@/lib/utils";
import { consumeProgressStream } from "@/lib/streamProgress";

export default function SetupPage() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [mode, setMode] = useState<'register' | 'restore'>('register');
  const [formData, setFormData] = useState({
    name: "",
    email: "",
    password: "",
    confirmPassword: "",
    bootstrapSecret: "",
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

    if (!formData.bootstrapSecret.trim()) {
      setError("Bootstrap secret is required (from backend logs or data/.bootstrap-secret)");
      return;
    }

    setLoading(true);
    try {
      const res = await fetch(`${API_URL}/api/auth/register`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "x-bootstrap-secret": formData.bootstrapSecret.trim(),
        },
        body: JSON.stringify({
          name: formData.name,
          email: formData.email,
          password: formData.password,
          bootstrapSecret: formData.bootstrapSecret.trim(),
        }),
      });

      const data = await res.json();

      if (!res.ok) {
        throw new Error(data.error || "Registration failed");
      }

      // Store token and user in global auth state
      auth.login(data.token, data.user);

      // Redirect to dashboard
      navigate("/");
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
      if (!formData.bootstrapSecret.trim()) {
        setError("Bootstrap secret is required for restore (from backend logs or data/.bootstrap-secret)");
        setRestoring(false);
        return;
      }

      const upload = new FormData();
      upload.append('backup', file);

      // Setup token requires bootstrap secret — never publicly retrievable
      let headers: HeadersInit = {
        'x-bootstrap-secret': formData.bootstrapSecret.trim(),
      };
      try {
        const tokenRes = await fetch(`${API_URL}/api/auth/setup-token`, {
          headers: { 'x-bootstrap-secret': formData.bootstrapSecret.trim() },
        });
        if (tokenRes.ok) {
          const tokenData = await tokenRes.json();
          if (tokenData.setupToken) {
            headers['x-setup-token'] = tokenData.setupToken;
          }
        } else {
          const errData = await tokenRes.json().catch(() => ({}));
          throw new Error(errData.error || 'Invalid bootstrap secret');
        }
      } catch (e: any) {
        setError(e.message || 'Failed to authorize restore');
        setRestoring(false);
        return;
      }

      const res = await fetch(`${API_URL}/api/backup/restore-upload`, {
        method: 'POST',
        headers,
        body: upload,
      });

      const result = await consumeProgressStream(res, (line) => {
        if (line.trim()) setRestoreProgress((prev) => [...prev, line]);
      });

      if (!result.ok) {
        setError(result.error || "Restore failed");
        return;
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

  const logo = (
    <div className="mb-6 flex items-center justify-center gap-2">
      <span className="flex size-7 items-center justify-center rounded-md bg-brand">
        <Container className="size-4 text-brand-foreground" />
      </span>
      <span className="text-[15px] font-semibold text-foreground">Docklift</span>
    </div>
  );

  const labelClass = "mb-1.5 block text-[13px] font-medium";
  const required = <span className="text-subtle">*</span>;
  const errorBox = error ? (
    <div className="mb-4 flex items-start gap-2 rounded-md border border-danger/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5 text-foreground">
      <AlertCircle className="mt-0.5 size-4 shrink-0 text-danger" />
      {error}
    </div>
  ) : null;
  const revealButton =
    "absolute right-1 top-1/2 flex size-7 -translate-y-1/2 items-center justify-center rounded-sm text-subtle transition-colors duration-150 hover:text-foreground pointer-coarse:size-9";

  // Show restore progress/complete view
  if (restoring || restoreComplete) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background px-4 pt-10 pb-[12vh]">
        <div className="w-full max-w-2xl">
          {logo}
          <Card className="p-5 sm:p-6">
            <div className="mb-4 flex items-start gap-3">
              {restoring ? (
                <Loader2 className="mt-1 size-4 shrink-0 animate-spin text-subtle" />
              ) : (
                <Check className="mt-1 size-4 shrink-0 text-ok" />
              )}
              <div>
                <h1 className="text-[15px] font-semibold leading-6">
                  {restoreComplete ? 'Restore complete' : 'Restoring backup…'}
                </h1>
                <p className="text-[13px] leading-5 text-muted-foreground">
                  {restoreComplete ? 'Your Docklift instance has been restored.' : 'Please wait while your backup is being restored.'}
                </p>
              </div>
            </div>

            <div className="mb-4 max-h-[400px] overflow-y-auto rounded-lg border border-border bg-[#0f1215] p-3 font-mono text-[12.5px] leading-5 text-white/85">
              {restoreProgress.map((line, i) => (
                <div key={i} className="whitespace-pre-wrap">{line}</div>
              ))}
              {restoring && <span className="text-white/50">▌</span>}
            </div>

            {restoreComplete && (
              <div className="space-y-4">
                <div className="rounded-md border border-ok/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5">
                  <p className="font-medium text-foreground">Restore successful.</p>
                  <p className="text-muted-foreground">
                    You can now sign in with your existing credentials from the backup.
                  </p>
                </div>
                <Button
                  size="lg"
                  onClick={() => navigate('/sign-in')}
                  className="w-full sm:w-auto"
                >
                  Go to sign in
                </Button>
              </div>
            )}
          </Card>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4 pt-10 pb-[12vh]">
      <div className="w-full max-w-[360px]">
        {logo}

        {mode === 'register' ? (
          <Card className="p-5 sm:p-6">
            <div className="mb-5">
              <h1 className="text-xl font-semibold leading-7 tracking-[-0.01em]">Create an account</h1>
              <p className="mt-0.5 text-[13.5px] leading-5 text-muted-foreground">
                This user will be the root user (full admin access).
              </p>
            </div>

            {errorBox}

            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className={labelClass}>
                  Name {required}
                </label>
                <Input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  placeholder="Your name"
                  required
                />
              </div>

              <div>
                <label className={labelClass}>
                  Email {required}
                </label>
                <Input
                  type="email"
                  value={formData.email}
                  onChange={(e) => setFormData({ ...formData, email: e.target.value })}
                  placeholder="admin@example.com"
                  required
                />
              </div>

              <div>
                <label className={labelClass}>
                  Password {required}
                </label>
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
                    className={revealButton}
                  >
                    {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </button>
                </div>
                {formData.password.length > 0 && formData.password.length < 8 && (
                  <p className="mt-1.5 text-xs text-muted-foreground">Password must be at least 8 characters</p>
                )}
              </div>

              <div>
                <label className={labelClass}>
                  Confirm password {required}
                </label>
                <div className="relative">
                  <Input
                    type={showConfirmPassword ? "text" : "password"}
                    value={formData.confirmPassword}
                    onChange={(e) => setFormData({ ...formData, confirmPassword: e.target.value })}
                    placeholder="••••••••"
                    required
                    className="pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                    className={revealButton}
                  >
                    {showConfirmPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </button>
                </div>
                {formData.confirmPassword && !passwordsMatch && (
                  <p className="mt-1.5 text-xs text-danger">Passwords do not match</p>
                )}
              </div>

              <div>
                <label className={labelClass}>
                  Bootstrap secret {required}
                </label>
                <Input
                  type="password"
                  value={formData.bootstrapSecret}
                  onChange={(e) => setFormData({ ...formData, bootstrapSecret: e.target.value })}
                  placeholder="From docker logs / data/.bootstrap-secret"
                  required
                  autoComplete="off"
                  className="font-mono placeholder:font-sans"
                />
                <p className="mt-1.5 text-xs leading-4 text-muted-foreground">
                  Shown once in backend logs on first start. Prevents remote claim of a fresh install.
                </p>
              </div>

              <Button
                type="submit"
                size="lg"
                disabled={loading || !isPasswordValid || !passwordsMatch || !formData.name || !formData.email || !formData.bootstrapSecret.trim()}
                className="w-full"
              >
                {loading ? <Loader2 className="animate-spin" /> : null}
                Register
              </Button>
            </form>

            <div className="my-5 flex items-center gap-3 text-xs text-subtle">
              <div className="h-px flex-1 bg-border" />
              or
              <div className="h-px flex-1 bg-border" />
            </div>

            <Button
              type="button"
              variant="outline"
              size="lg"
              onClick={() => setMode('restore')}
              className="w-full"
            >
              <RotateCcw className="text-subtle" />
              Restore from backup
            </Button>

            <p className="mt-5 text-center text-[13px] text-muted-foreground">
              Already registered?{" "}
              <Link to="/sign-in" className="rounded-sm font-medium text-brand underline-offset-4 hover:underline">
                Sign in
              </Link>
            </p>
          </Card>
        ) : (
          <Card className="p-5 sm:p-6">
            <button
              onClick={() => setMode('register')}
              className="mb-4 inline-flex items-center gap-1.5 rounded-sm text-[13px] text-muted-foreground transition-colors duration-150 hover:text-foreground"
            >
              <ArrowLeft className="size-4 text-subtle" />
              Back to registration
            </button>

            <div className="mb-5">
              <h1 className="text-xl font-semibold leading-7 tracking-[-0.01em]">Restore from backup</h1>
              <p className="mt-0.5 text-[13.5px] leading-5 text-muted-foreground">
                Restore your Docklift instance from a backup file.
              </p>
            </div>

            {errorBox}

            <div className="mb-4">
              <label className={labelClass}>
                Bootstrap secret {required}
              </label>
              <Input
                type="password"
                value={formData.bootstrapSecret}
                onChange={(e) => setFormData({ ...formData, bootstrapSecret: e.target.value })}
                placeholder="From docker logs / data/.bootstrap-secret"
                autoComplete="off"
                className="font-mono placeholder:font-sans"
              />
            </div>

            <div
              className={`rounded-lg border border-dashed px-4 py-6 transition-colors duration-150 ${
                isDragging
                  ? 'border-brand bg-brand/5'
                  : 'border-input bg-secondary/40'
              }`}
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
            >
              <div className="flex flex-col items-center gap-3 text-center">
                <FileUp className={`size-5 ${isDragging ? 'text-brand' : 'text-subtle'}`} />
                <div>
                  <p className="text-sm font-medium">
                    {isDragging ? 'Drop your backup file here' : 'Drag and drop or select a backup file'}
                  </p>
                  <p className="mt-0.5 text-[13px] text-muted-foreground">
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
                    disabled={restoring}
                    asChild
                  >
                    <span>
                      {restoring ? (
                        <Loader2 className="animate-spin" />
                      ) : (
                        <Upload />
                      )}
                      Select backup file
                    </span>
                  </Button>
                </label>
              </div>
            </div>

            <div className="mt-5 text-[13px] leading-5">
              <p className="font-medium">Backup will restore</p>
              <ul className="mt-1 list-inside list-disc space-y-0.5 text-muted-foreground marker:text-subtle">
                <li>All user accounts and credentials</li>
                <li>Projects and deployments</li>
                <li>Environment variables</li>
                <li>Nginx configurations</li>
                <li>GitHub App settings</li>
              </ul>
            </div>

            <div className="mt-4 rounded-md border border-info/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5">
              <p className="font-medium text-foreground">After restore</p>
              <ul className="mt-1 list-inside list-disc space-y-0.5 text-muted-foreground marker:text-subtle">
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
