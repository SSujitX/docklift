// Settings page - GitHub integration, server config, port allocation, Docker, and domain management

import { useState, useEffect, Suspense } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { PageHeader } from "@/components/shell/PageHeader";
import { useShell } from "@/components/shell/ShellContext";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Loader2, Check, X, Plus, Trash2, Copy, Download, RotateCcw, Upload, FileUp, Eye, EyeOff, ExternalLink } from "lucide-react";
import { GithubIcon } from "@/components/icons/GithubIcon";
import { toast } from "sonner";
import { API_URL, cn, copyToClipboard } from "@/lib/utils";
import { GitHubConnect } from "@/components/GitHubConnect";
import { authFetch, startGithubInstallAndNavigate } from "@/lib/auth";
import { useAuth } from "@/components/AuthProvider";
import type { SslInfo } from "@/components/SslStatusBadge";
import { DnsGuideCard } from "@/components/domains/DnsGuideCard";
import { PanelDomainCard } from "@/components/domains/PanelDomainCard";
import type { PanelAccess } from "@/lib/panelAccess";
import { consumeProgressStream } from "@/lib/streamProgress";
import { SETTINGS_TAB_IDS, settingsSectionLabel } from "@/lib/settingsNav";

interface GitHubStatus {
  connected: boolean;
  username?: string;
  avatar_url?: string;
  name?: string;
  app_name?: string | null;
  app_slug?: string | null;
  error?: string;
}

interface DomainConfig {
  domain: string;
  port: number;
  ssl?: SslInfo;
  access?: PanelAccess;
}

interface GitHubInstallation {
  id: number;
  login: string;
  avatar_url: string;
  type: 'User' | 'Organization';
}

interface BackupInfo {
  filename: string;
  size: number;
  created_at: string;
}

function SettingsContent() {
  const [githubStatus, setGithubStatus] = useState<GitHubStatus | null>(null);
  const [githubInstallations, setGithubInstallations] = useState<GitHubInstallation[]>([]);
  const [loading, setLoading] = useState(true);
  const [disconnecting, setDisconnecting] = useState(false);
  const [showGitHubConnect, setShowGitHubConnect] = useState(false);
  const [activeTab, setActiveTab] = useState('profile');
  
  // Profile State
  const { user, updateUser, login } = useAuth();
  const [profileData, setProfileData] = useState({ name: user?.name || '', email: user?.email || '' });
  const [updatingProfile, setUpdatingProfile] = useState(false);
  
  // Password State
  const [passwordData, setPasswordData] = useState({ currentPassword: '', newPassword: '', confirmPassword: '' });
  const [updatingPassword, setUpdatingPassword] = useState(false);
  const [showCurrentPassword, setShowCurrentPassword] = useState(false);
  const [showNewPassword, setShowNewPassword] = useState(false);
  
  // Domain State
  const [domains, setDomains] = useState<DomainConfig[]>([]);
  const [loadingDomains, setLoadingDomains] = useState(false);
  const [serverIP, setServerIP] = useState<string>('...');
  const [acmeEmail, setAcmeEmail] = useState('');
  const [savingAcmeEmail, setSavingAcmeEmail] = useState(false);

  // Backup State
  const [backups, setBackups] = useState<BackupInfo[]>([]);
  const [uploadedFiles, setUploadedFiles] = useState<BackupInfo[]>([]);
  const [loadingBackups, setLoadingBackups] = useState(false);
  const [loadingUploads, setLoadingUploads] = useState(false);
  const [creatingBackup, setCreatingBackup] = useState(false);
  const [backupProgress, setBackupProgress] = useState<string[]>([]);
  const [showBackupProgress, setShowBackupProgress] = useState(false);
  const [restoringBackup, setRestoringBackup] = useState(false);
  const [restoreProgress, setRestoreProgress] = useState<string[]>([]);
  const [showRestoreProgress, setShowRestoreProgress] = useState(false);
  const [backupToDelete, setBackupToDelete] = useState<string | null>(null);
  const [showDeleteBackupConfirm, setShowDeleteBackupConfirm] = useState(false);
  const [deletingBackup, setDeletingBackup] = useState(false);
  const [uploadingBackup, setUploadingBackup] = useState(false);
  const [showUploadRestoreConfirm, setShowUploadRestoreConfirm] = useState(false);
  const [selectedUploadFile, setSelectedUploadFile] = useState<File | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [uploadToDelete, setUploadToDelete] = useState<string | null>(null);
  const [showDeleteUploadConfirm, setShowDeleteUploadConfirm] = useState(false);
  const [deletingUpload, setDeletingUpload] = useState(false);
  const [uploadToRestore, setUploadToRestore] = useState<string | null>(null);
  const [showRestoreUploadConfirm, setShowRestoreUploadConfirm] = useState(false);
  const [backupName, setBackupName] = useState('');
  const [restorePassword, setRestorePassword] = useState('');

  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { setBreadcrumbLeaf } = useShell();

  // Sync activeTab with URL query parameter
  useEffect(() => {
    const tab = searchParams.get("tab");

    if (tab && SETTINGS_TAB_IDS.includes(tab)) {
      setActiveTab(tab);
    } else if (!tab) {
      setActiveTab("profile");
    }

    if (searchParams.get("github") === "connected") {
      toast.success("GitHub account connected");
      navigate("/settings?tab=github", { replace: true });
    }
  }, [searchParams, navigate]);

  useEffect(() => {
    setBreadcrumbLeaf(settingsSectionLabel(activeTab));
  }, [activeTab, setBreadcrumbLeaf]);

  // Fetch data based on active tab
  useEffect(() => {
    if (activeTab === 'github') fetchGitHubStatus();
    if (activeTab === 'domain') {
      fetchDomains();
      fetchServerIP();
    }
    if (activeTab === 'server') fetchServerIP();
    if (activeTab === 'backup') fetchBackups();
    if (activeTab === 'restore') fetchUploadedFiles();
  }, [activeTab]);

  useEffect(() => {
    if (user) {
      setProfileData({ name: user.name || '', email: user.email || '' });
    }
  }, [user]);

  const fetchServerIP = async () => {
    try {
      const res = await authFetch(`${API_URL}/api/system/ip`);
      if (res.ok) {
        const data = await res.json();
        setServerIP(data.ip || 'N/A');
      }
    } catch {
      setServerIP('N/A');
    }
  };

  const fetchGitHubStatus = async () => {
    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 5000); // 5s timeout
      
      const res = await authFetch(`${API_URL}/api/github/status`, {
        signal: controller.signal,
      });
      clearTimeout(timeoutId);
      const data = await res.json();
      setGithubStatus(data);

      // Also fetch all installations if connected
      if (data.connected) {
        try {
          const instRes = await authFetch(`${API_URL}/api/github/installations`);
          if (instRes.ok) {
            const instData = await instRes.json();
            setGithubInstallations(instData.installations || []);
          }
        } catch {
          // Ignore installation fetch errors
        }
      }
    } catch {
      setGithubStatus({ connected: false });
    } finally {
      setLoading(false);
    }
  };

  const handleConnectGitHub = () => {
    setShowGitHubConnect(true);
  };

  const handleAddGithubAccount = async () => {
    try {
      await startGithubInstallAndNavigate(window.location.href);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to start GitHub install");
    }
  };

  const handleDisconnectGitHub = async () => {
    setDisconnecting(true);
    try {
      await authFetch(`${API_URL}/api/github/disconnect`, { method: "POST" });
      setGithubStatus({ connected: false });
      toast.success("GitHub disconnected");
    } catch {
      toast.error("Failed to disconnect");
    } finally {
      setDisconnecting(false);
    }
  };

  const fetchDomains = async (opts?: { silent?: boolean }): Promise<boolean> => {
    if (!opts?.silent) setLoadingDomains(true);
    try {
      const res = await authFetch(`${API_URL}/api/domains`);
      if (res.ok) {
        const data = await res.json();
        setDomains(data);
      } else {
        if (!opts?.silent) toast.error("Failed to load domains");
        return false;
      }
      const emailRes = await authFetch(`${API_URL}/api/domains/ssl/email`);
      if (emailRes.ok) {
        const emailData = await emailRes.json();
        setAcmeEmail(emailData.email || '');
      }
      return true;
    } catch (error) {
      if (!opts?.silent) toast.error("Failed to load domains");
      return false;
    } finally {
      if (!opts?.silent) setLoadingDomains(false);
    }
  };

  const handleSaveAcmeEmail = async () => {
    setSavingAcmeEmail(true);
    try {
      const res = await authFetch(`${API_URL}/api/domains/ssl/email`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: acmeEmail }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || "Failed to save SSL email");
      toast.success("ACME email saved");
    } catch (error: any) {
      toast.error(error.message);
    } finally {
      setSavingAcmeEmail(false);
    }
  };

  const handleUpdateProfile = async (e: React.FormEvent) => {
    e.preventDefault();
    setUpdatingProfile(true);
    try {
      const res = await authFetch(`${API_URL}/api/auth/profile`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: profileData.name,
          email: profileData.email
        })
      });

      const data = await res.json();
      if (!res.ok) throw new Error(data.error || "Failed to update profile");
      
      updateUser(data.user);
      toast.success("Profile updated successfully");
    } catch (error: any) {
      toast.error(error.message);
    } finally {
      setUpdatingProfile(false);
    }
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (passwordData.newPassword !== passwordData.confirmPassword) {
      toast.error("Passwords do not match");
      return;
    }

    setUpdatingPassword(true);
    try {
      const res = await authFetch(`${API_URL}/api/auth/change-password`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          currentPassword: passwordData.currentPassword,
          newPassword: passwordData.newPassword
        })
      });

      const data = await res.json();
      if (!res.ok) throw new Error(data.error || "Failed to change password");

      // Password change invalidates old JWTs — keep this session alive with the new token
      if (data.token && user) {
        login(data.token, user);
      }
      
      toast.success("Password changed successfully");
      setPasswordData({ currentPassword: '', newPassword: '', confirmPassword: '' });
    } catch (error: any) {
      toast.error(error.message);
    } finally {
      setUpdatingPassword(false);
    }
  };

  // Backup Functions
  const fetchBackups = async () => {
    setLoadingBackups(true);
    try {
      const res = await authFetch(`${API_URL}/api/backup`);
      if (res.ok) {
        const data = await res.json();
        setBackups(data);
      }
    } catch (error) {
      toast.error("Failed to load backups");
    } finally {
      setLoadingBackups(false);
    }
  };

  const fetchUploadedFiles = async () => {
    setLoadingUploads(true);
    try {
      const res = await authFetch(`${API_URL}/api/backup/uploads`);
      if (res.ok) {
        const data = await res.json();
        setUploadedFiles(data);
      }
    } catch (error) {
      toast.error("Failed to load uploaded files");
    } finally {
      setLoadingUploads(false);
    }
  };

  const handleDeleteUpload = async () => {
    if (!uploadToDelete) return;

    setDeletingUpload(true);
    try {
      const res = await authFetch(`${API_URL}/api/backup/uploads/${uploadToDelete}`, {
        method: 'DELETE',
      });

      if (!res.ok) throw new Error("Failed to delete file");

      toast.success("File deleted");
      setShowDeleteUploadConfirm(false);
      setUploadToDelete(null);
      fetchUploadedFiles();
    } catch (error) {
      toast.error("Failed to delete file");
    } finally {
      setDeletingUpload(false);
    }
  };

  const handleRestoreFromUpload = async () => {
    if (!uploadToRestore) return;
    if (!restorePassword.trim()) {
      toast.error("Enter your account password to confirm restore");
      return;
    }

    setRestoringBackup(true);
    setRestoreProgress([]);
    setShowRestoreProgress(true);
    setShowRestoreUploadConfirm(false);

    try {
      const res = await authFetch(`${API_URL}/api/backup/restore-from-upload/${uploadToRestore}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: restorePassword }),
      });

      const result = await consumeProgressStream(res, (line) => {
        if (line.trim()) setRestoreProgress((prev) => [...prev, line]);
      });

      if (!result.ok) {
        toast.error(result.error || "Failed to restore from uploaded file");
        return;
      }

      toast.success("Backup restored successfully");
      setRestorePassword('');
      fetchUploadedFiles();
    } catch (error) {
      toast.error("Failed to restore from uploaded file");
    } finally {
      setRestoringBackup(false);
      setUploadToRestore(null);
    }
  };

  const handleCreateBackup = async () => {
    setCreatingBackup(true);
    setBackupProgress([]);
    setShowBackupProgress(true);

    try {
      const res = await authFetch(`${API_URL}/api/backup/create`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: backupName.trim() || undefined }),
      });

      const result = await consumeProgressStream(res, (line) => {
        if (line.trim()) setBackupProgress((prev) => [...prev, line]);
      });

      if (!result.ok) {
        toast.error(result.error || "Failed to create backup");
        return;
      }

      toast.success("Backup created successfully");
      setBackupName('');
      fetchBackups();
    } catch (error) {
      toast.error("Failed to create backup");
    } finally {
      setCreatingBackup(false);
    }
  };

  const handleDeleteBackup = async () => {
    if (!backupToDelete) return;

    setDeletingBackup(true);
    try {
      const res = await authFetch(`${API_URL}/api/backup/${backupToDelete}`, {
        method: 'DELETE',
      });

      if (!res.ok) throw new Error("Failed to delete backup");

      toast.success("Backup deleted");
      setShowDeleteBackupConfirm(false);
      setBackupToDelete(null);
      fetchBackups();
    } catch (error) {
      toast.error("Failed to delete backup");
    } finally {
      setDeletingBackup(false);
    }
  };

  const handleDownloadBackup = async (filename: string) => {
    try {
      const res = await authFetch(`${API_URL}/api/backup/download/${filename}`);
      if (!res.ok) throw new Error('Download failed');
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch {
      toast.error('Failed to download backup');
    }
  };

  const formatBytes = (bytes: number): string => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const handleUploadFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      if (!file.name.endsWith('.zip')) {
        toast.error("Please select a .zip backup file");
        return;
      }
      setSelectedUploadFile(file);
      setShowUploadRestoreConfirm(true);
    }
    // Reset input
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

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);

    const file = e.dataTransfer.files?.[0];
    if (file) {
      if (!file.name.endsWith('.zip')) {
        toast.error("Please drop a .zip backup file");
        return;
      }
      setSelectedUploadFile(file);
      setShowUploadRestoreConfirm(true);
    }
  };

  const handleUploadRestore = async () => {
    if (!selectedUploadFile) return;
    if (!restorePassword.trim()) {
      toast.error("Enter your account password to confirm restore");
      return;
    }

    setUploadingBackup(true);
    setRestoreProgress([]);
    setShowRestoreProgress(true);
    setShowUploadRestoreConfirm(false);

    try {
      const formData = new FormData();
      formData.append('backup', selectedUploadFile);
      formData.append('password', restorePassword);

      const res = await authFetch(`${API_URL}/api/backup/restore-upload`, {
        method: 'POST',
        body: formData,
      });

      const result = await consumeProgressStream(res, (line) => {
        if (line.trim()) setRestoreProgress((prev) => [...prev, line]);
      });

      if (!result.ok) {
        toast.error(result.error || "Failed to restore from uploaded backup");
        return;
      }

      toast.success("Backup restored successfully");
      setRestorePassword('');
      fetchBackups();
    } catch (error) {
      toast.error("Failed to restore from uploaded backup");
    } finally {
      setUploadingBackup(false);
      setSelectedUploadFile(null);
    }
  };

  return (
    <>
      <div>
        <PageHeader
          title={settingsSectionLabel(activeTab)}
          description={TAB_DESCRIPTIONS[activeTab] ?? "Settings for this Docklift server."}
        />

        <div className="min-w-0 space-y-5">
            {/* Profile */}
            {activeTab === 'profile' && (
              <>
                <Panel title="Account profile" description="Name and email for this Docklift account.">
                  <form onSubmit={handleUpdateProfile}>
                    <PanelBody>
                      <div className="grid gap-4 sm:grid-cols-2">
                        <Field label="Full name">
                          <Input
                            value={profileData.name}
                            onChange={(e) => setProfileData({ ...profileData, name: e.target.value })}
                            placeholder="John Doe"
                          />
                        </Field>
                        <Field label="Email address">
                          <Input
                            type="email"
                            value={profileData.email}
                            onChange={(e) => setProfileData({ ...profileData, email: e.target.value })}
                            placeholder="john@example.com"
                          />
                        </Field>
                      </div>
                    </PanelBody>
                    <PanelFooter>
                      <Button type="submit" disabled={updatingProfile}>
                        {updatingProfile && <Loader2 className="animate-spin" />}
                        Save profile
                      </Button>
                    </PanelFooter>
                  </form>
                </Panel>

                <Panel title="Password" description="Change the password used to sign in. This session stays signed in.">
                  <form onSubmit={handleChangePassword}>
                    <PanelBody>
                      <div className="space-y-4">
                        <Field label="Current password">
                          <div className="relative">
                            <Input
                              type={showCurrentPassword ? "text" : "password"}
                              value={passwordData.currentPassword}
                              onChange={(e) => setPasswordData({ ...passwordData, currentPassword: e.target.value })}
                              placeholder="Current password"
                              className="pr-10"
                            />
                            <button
                              type="button"
                              onClick={() => setShowCurrentPassword(!showCurrentPassword)}
                              className="absolute right-1 top-1/2 inline-flex size-7 -translate-y-1/2 items-center justify-center rounded-md text-subtle transition-colors duration-150 hover:bg-secondary hover:text-foreground"
                            >
                              {showCurrentPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                              <span className="sr-only">{showCurrentPassword ? "Hide password" : "Show password"}</span>
                            </button>
                          </div>
                        </Field>

                        <div className="grid gap-4 sm:grid-cols-2">
                          <Field label="New password">
                            <div className="relative">
                              <Input
                                type={showNewPassword ? "text" : "password"}
                                value={passwordData.newPassword}
                                onChange={(e) => setPasswordData({ ...passwordData, newPassword: e.target.value })}
                                placeholder="New password"
                                className="pr-10"
                              />
                              <button
                                type="button"
                                onClick={() => setShowNewPassword(!showNewPassword)}
                                className="absolute right-1 top-1/2 inline-flex size-7 -translate-y-1/2 items-center justify-center rounded-md text-subtle transition-colors duration-150 hover:bg-secondary hover:text-foreground"
                              >
                                {showNewPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                                <span className="sr-only">{showNewPassword ? "Hide password" : "Show password"}</span>
                              </button>
                            </div>
                          </Field>
                          <Field label="Confirm new password">
                            <Input
                              type="password"
                              value={passwordData.confirmPassword}
                              onChange={(e) => setPasswordData({ ...passwordData, confirmPassword: e.target.value })}
                              placeholder="Confirm password"
                            />
                          </Field>
                        </div>
                      </div>
                    </PanelBody>
                    <PanelFooter>
                      <Button type="submit" disabled={updatingPassword || !passwordData.newPassword}>
                        {updatingPassword && <Loader2 className="animate-spin" />}
                        Change password
                      </Button>
                    </PanelFooter>
                  </form>
                </Panel>
              </>
            )}

            {/* GitHub Tab */}
            {activeTab === 'github' && (
              <Panel
                title="GitHub App"
                description="Connect to automatically deploy public and private repositories."
              >
                <dl className="divide-y divide-border">
                  <DefRow label="Status">
                    {loading ? (
                      <span className="inline-flex items-center gap-2 text-muted-foreground">
                        <Loader2 className="size-3.5 animate-spin" />
                        Checking…
                      </span>
                    ) : githubStatus?.connected ? (
                      <StatusDot tone="ok">Connected</StatusDot>
                    ) : (
                      <StatusDot tone="none">Not connected</StatusDot>
                    )}
                  </DefRow>

                  {githubStatus?.app_name && (
                    <DefRow label="App">
                      {githubStatus.app_slug ? (
                        <a
                          href={`https://github.com/apps/${githubStatus.app_slug}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center gap-1 font-mono text-[13px] text-brand underline-offset-4 hover:underline"
                        >
                          {githubStatus.app_name}
                          <ExternalLink className="size-3.5" />
                        </a>
                      ) : (
                        <span className="font-mono text-[13px]">{githubStatus.app_name}</span>
                      )}
                    </DefRow>
                  )}

                  {!loading && githubStatus?.connected && githubInstallations.length > 0 && (
                    <DefRow label="Accounts">
                      <ul className="space-y-2">
                        {githubInstallations.map((inst) => (
                          <li key={inst.id} className="flex min-w-0 items-center gap-2.5">
                            {inst.avatar_url ? (
                              <img src={inst.avatar_url} alt={inst.login} className="size-5 shrink-0 rounded-sm" />
                            ) : (
                              <span className="flex size-5 shrink-0 items-center justify-center rounded-sm bg-secondary text-xs font-semibold text-muted-foreground">
                                {inst.login?.charAt(0).toUpperCase()}
                              </span>
                            )}
                            <span className="truncate font-mono text-[13px]">@{inst.login}</span>
                            <span className="shrink-0 text-xs text-muted-foreground">
                              {inst.type === 'Organization' ? 'Organization' : 'Personal'}
                            </span>
                          </li>
                        ))}
                      </ul>
                    </DefRow>
                  )}
                </dl>

                {!loading && (
                  <PanelFooter>
                    {githubStatus?.connected ? (
                      <>
                        <Button
                          variant="outline"
                          className="text-danger"
                          onClick={handleDisconnectGitHub}
                          disabled={disconnecting}
                        >
                          {disconnecting ? <Loader2 className="animate-spin" /> : <X />}
                          Disconnect
                        </Button>
                        <Button
                          variant="outline"
                          title="Add another account or organization — repos will be combined"
                          onClick={handleAddGithubAccount}
                        >
                          <Plus className="text-subtle" />
                          Add account
                        </Button>
                      </>
                    ) : (
                      <Button onClick={handleConnectGitHub}>
                        <GithubIcon className="size-4" />
                        Connect GitHub
                      </Button>
                    )}
                  </PanelFooter>
                )}
                <GitHubConnect
                  open={showGitHubConnect}
                  onOpenChange={setShowGitHubConnect}
                  onConnected={fetchGitHubStatus}
                />
              </Panel>
            )}

            {/* Server Tab */}
            {activeTab === 'server' && (
              <Panel title="Addresses" description="Where this Docklift installation answers and stores its files.">
                <dl className="divide-y divide-border">
                  <CopyRow
                    label="Server IP address"
                    value={serverIP}
                    help="Your server's public IP address."
                    onCopy={() => {
                      copyToClipboard(serverIP);
                      toast.success('IP copied');
                    }}
                  />
                  <CopyRow
                    label="Docklift panel URL"
                    value={`http://${serverIP}:8080`}
                    help="Access Docklift at this URL (frontend port 8080)."
                    onCopy={() => {
                      copyToClipboard(`http://${serverIP}:8080`);
                      toast.success('URL copied');
                    }}
                  />
                  <CopyRow
                    label="Backend API URL"
                    value={`http://${serverIP}:4000`}
                    help="Backend API endpoint (port 4000)."
                    onCopy={() => {
                      copyToClipboard(`http://${serverIP}:4000`);
                      toast.success('URL copied');
                    }}
                  />
                  <CopyRow
                    label="Deployments directory"
                    value="/deployments"
                    help="Absolute path where project files are stored on the host."
                    onCopy={() => {
                      copyToClipboard('/deployments');
                      toast.success('Path copied');
                    }}
                  />
                </dl>
              </Panel>
            )}

            {/* Port Tab */}
            {activeTab === 'port' && (
              <Panel title="Port allocation" description="The range of host ports available for deployments.">
                <dl className="divide-y divide-border">
                  <DefRow label="Start port">
                    <span className="font-mono text-[13px]">3001</span>
                  </DefRow>
                  <DefRow label="End port">
                    <span className="font-mono text-[13px]">3100</span>
                  </DefRow>
                </dl>
                <p className="border-t border-border px-4 py-3 text-[13px] leading-5 text-muted-foreground sm:px-5">
                  Docklift automatically assigns the next available port from this pool when creating
                  new deployments.
                </p>
              </Panel>
            )}

            {/* Docker Tab */}
            {activeTab === 'docker' && (
              <Panel title="Docker network" description="Container networking used by every application.">
                <dl className="divide-y divide-border">
                  <DefRow
                    label="Network bridge"
                    help="All application containers are attached to this bridge network to allow internal communication."
                  >
                    <span className="font-mono text-[13px]">docklift_network</span>
                  </DefRow>
                </dl>
              </Panel>
            )}

            {/* Domain Tab — same layout/copy as project Domain (guide + card + activity) */}
            {activeTab === 'domain' && (
              <>
                <DnsGuideCard serverIP={serverIP} />

                <Panel
                  title="Certificate email"
                  description="Let's Encrypt / ACME account email. Optional."
                >
                  <PanelBody>
                    <Field
                      label="Email"
                      help="Optional. Used for certificate expiry notices. If empty, Docklift uses your admin account email."
                    >
                      <Input
                        type="email"
                        value={acmeEmail}
                        onChange={(e) => setAcmeEmail(e.target.value)}
                        placeholder="admin@example.com"
                      />
                    </Field>
                  </PanelBody>
                  <PanelFooter>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => void handleSaveAcmeEmail()}
                      disabled={savingAcmeEmail || !acmeEmail.includes("@")}
                    >
                      {savingAcmeEmail && <Loader2 className="animate-spin" />}
                      Save
                    </Button>
                  </PanelFooter>
                </Panel>

                {loadingDomains ? (
                  <Panel title="Panel domain" description="Maps a hostname to this Docklift dashboard, wires the reverse proxy, and requests a certificate.">
                    <div className="flex justify-center py-14">
                      <Loader2 className="size-5 animate-spin text-subtle" />
                    </div>
                  </Panel>
                ) : (
                  <PanelDomainCard
                    domains={domains}
                    serverIP={serverIP}
                    onUpdate={() => fetchDomains({ silent: true })}
                  />
                )}
              </>
            )}

            {/* Backup Tab */}
            {activeTab === 'backup' && (
              <>
                <Panel title="Create backup" description="A full system backup with all your data, saved on this server.">
                  <PanelBody>
                    <div className="space-y-5">
                      <Field
                        label="Name"
                        optional
                        help={
                          <>
                            Output:{" "}
                            <span className="font-mono text-foreground">
                              {backupName.trim() ? `${backupName.trim().replace(/[^a-zA-Z0-9_-]/g, '-').slice(0, 50)}` : 'docklift'}-backup-[timestamp].zip
                            </span>
                          </>
                        }
                      >
                        <Input
                          placeholder="e.g. before-migration, v1.0"
                          value={backupName}
                          onChange={(e) => setBackupName(e.target.value)}
                          disabled={creatingBackup}
                        />
                      </Field>

                      <div className="text-[13px] leading-5">
                        <p className="font-medium">Backups include</p>
                        <ul className="mt-1 list-disc space-y-0.5 pl-5 text-muted-foreground marker:text-subtle">
                          <li>Database (projects, deployments, users, settings, env vars)</li>
                          <li>All project files from <span className="font-mono text-xs">/deployments/</span></li>
                          <li>Nginx configurations</li>
                          <li>GitHub App key (if configured)</li>
                        </ul>
                      </div>

                      <Callout tone="info" title="Migration tip">
                        If you use a <span className="font-medium text-foreground">domain name</span> (e.g.
                        docklift.yourdomain.com) instead of an IP address for your GitHub App webhook URL,
                        migrating to a new server becomes seamless — just update DNS and restore. No need to
                        reconfigure the GitHub App.
                      </Callout>
                    </div>
                  </PanelBody>
                  <PanelFooter>
                    <Button onClick={handleCreateBackup} disabled={creatingBackup}>
                      {creatingBackup && <Loader2 className="animate-spin" />}
                      Create backup
                    </Button>
                  </PanelFooter>
                </Panel>

                {/* Server Backups List */}
                <Panel
                  title="Server backups"
                  description="Backups stored on this server."
                  meta={!loadingBackups && backups.length > 0 ? backups.length : undefined}
                >
                  {loadingBackups ? (
                    <div className="flex justify-center py-14">
                      <Loader2 className="size-5 animate-spin text-subtle" />
                    </div>
                  ) : backups.length > 0 ? (
                    <FileTable
                      nameHead="Backup"
                      dateHead="Created"
                      rows={backups}
                      formatBytes={formatBytes}
                      renderActions={(backup) => (
                        <>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="text-subtle"
                            onClick={() => handleDownloadBackup(backup.filename)}
                            title="Download"
                          >
                            <Download />
                            <span className="sr-only">Download</span>
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="text-subtle hover:text-danger"
                            onClick={() => {
                              setBackupToDelete(backup.filename);
                              setShowDeleteBackupConfirm(true);
                            }}
                            title="Delete"
                          >
                            <Trash2 />
                            <span className="sr-only">Delete</span>
                          </Button>
                        </>
                      )}
                    />
                  ) : (
                    <EmptyState title="No backups yet" line="Create your first backup above." />
                  )}
                </Panel>
              </>
            )}

            {/* Restore Tab */}
            {activeTab === 'restore' && (
              <>
                {/* Warning Banner */}
                <Callout tone="danger" title="Restore replaces all data">
                  Restoring from a backup will replace all current projects, deployments, settings,
                  users, and environment variables.
                </Callout>

                {/* Upload and Restore */}
                <Panel title="Restore from file" description="Upload a backup file from your computer.">
                  <PanelBody>
                    <div className="space-y-5">
                      <div
                        className={cn(
                          'flex flex-col items-center gap-3 rounded-md border border-dashed px-4 py-8 text-center transition-colors duration-150',
                          isDragging ? 'border-brand bg-brand/5' : 'border-input',
                        )}
                        onDragOver={handleDragOver}
                        onDragLeave={handleDragLeave}
                        onDrop={handleDrop}
                      >
                        <FileUp className={cn('size-5', isDragging ? 'text-brand' : 'text-subtle')} />
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
                            onChange={handleUploadFileSelect}
                            className="hidden"
                            disabled={uploadingBackup || restoringBackup}
                          />
                          <Button
                            type="button"
                            variant="outline"
                            disabled={uploadingBackup || restoringBackup}
                            asChild
                          >
                            <span>
                              {uploadingBackup ? (
                                <Loader2 className="animate-spin" />
                              ) : (
                                <Upload className="text-subtle" />
                              )}
                              Select backup file
                            </span>
                          </Button>
                        </label>
                      </div>

                      <div className="text-[13px] leading-5">
                        <p className="font-medium">After restore</p>
                        <ul className="mt-1 list-disc space-y-0.5 pl-5 text-muted-foreground marker:text-subtle">
                          <li>Sign in with credentials from the backup</li>
                          <li>Redeploy each project (containers need rebuilding)</li>
                          <li>Update DNS if the server IP changed</li>
                          <li>The GitHub App works automatically if it uses a domain-based webhook URL</li>
                        </ul>
                      </div>
                    </div>
                  </PanelBody>
                </Panel>

                {/* Uploaded Files List */}
                <Panel
                  title="Uploaded restore files"
                  description="Backup files uploaded to this server."
                  meta={!loadingUploads && uploadedFiles.length > 0 ? uploadedFiles.length : undefined}
                >
                  {loadingUploads ? (
                    <div className="flex justify-center py-14">
                      <Loader2 className="size-5 animate-spin text-subtle" />
                    </div>
                  ) : uploadedFiles.length > 0 ? (
                    <FileTable
                      nameHead="File"
                      dateHead="Uploaded"
                      rows={uploadedFiles}
                      formatBytes={formatBytes}
                      renderTag={(file) =>
                        file.filename.includes('.restored-') ? (
                          <span className="inline-flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                            <span aria-hidden="true" className="size-1.5 rounded-full bg-ok" />
                            Restored
                          </span>
                        ) : null
                      }
                      renderActions={(file) => (
                        <>
                          <Button
                            variant="outline"
                            size="sm"
                            className="text-danger"
                            onClick={() => {
                              setUploadToRestore(file.filename);
                              setShowRestoreUploadConfirm(true);
                            }}
                            title="Restore"
                            disabled={restoringBackup}
                          >
                            <RotateCcw />
                            <span className="max-sm:sr-only">Restore</span>
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="text-subtle hover:text-danger"
                            onClick={() => {
                              setUploadToDelete(file.filename);
                              setShowDeleteUploadConfirm(true);
                            }}
                            title="Delete"
                          >
                            <Trash2 />
                            <span className="sr-only">Delete</span>
                          </Button>
                        </>
                      )}
                    />
                  ) : (
                    <EmptyState
                      title="No uploaded files"
                      line="Upload a backup file above to restore."
                    />
                  )}
                </Panel>
              </>
            )}

        </div>
      </div>

      {/* Delete Backup Confirmation Dialog */}
      <Dialog open={showDeleteBackupConfirm} onOpenChange={setShowDeleteBackupConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete backup</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete{" "}
              <span className="font-mono text-foreground break-all">{backupToDelete}</span>? This
              action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeleteBackupConfirm(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteBackup}
              disabled={deletingBackup}
            >
              {deletingBackup && <Loader2 className="animate-spin" />}
              Delete backup
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Upload Restore Confirmation Dialog */}
      <Dialog open={showUploadRestoreConfirm} onOpenChange={setShowUploadRestoreConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Restore from uploaded backup</DialogTitle>
            <DialogDescription>
              Are you sure you want to restore from{" "}
              <span className="font-mono text-foreground break-all">{selectedUploadFile?.name}</span>?
              All current projects, deployments, and configurations will be replaced.
            </DialogDescription>
          </DialogHeader>
          <Callout tone="danger">This will replace all current data.</Callout>
          <Field label="Confirm with account password">
            <Input
              type="password"
              value={restorePassword}
              onChange={(e) => setRestorePassword(e.target.value)}
              placeholder="Your DockLift password"
              autoComplete="current-password"
            />
          </Field>
          <DialogFooter>
            <Button variant="outline" onClick={() => {
              setShowUploadRestoreConfirm(false);
              setSelectedUploadFile(null);
              setRestorePassword('');
            }}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleUploadRestore}
              disabled={uploadingBackup || !restorePassword.trim()}
            >
              {uploadingBackup && <Loader2 className="animate-spin" />}
              Restore backup
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Uploaded File Confirmation Dialog */}
      <Dialog open={showDeleteUploadConfirm} onOpenChange={setShowDeleteUploadConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete uploaded file</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete{" "}
              <span className="font-mono text-foreground break-all">{uploadToDelete}</span>? This
              action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeleteUploadConfirm(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteUpload}
              disabled={deletingUpload}
            >
              {deletingUpload && <Loader2 className="animate-spin" />}
              Delete file
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Restore from Uploaded File Confirmation Dialog */}
      <Dialog open={showRestoreUploadConfirm} onOpenChange={setShowRestoreUploadConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Restore from uploaded file</DialogTitle>
            <DialogDescription>
              Are you sure you want to restore from{" "}
              <span className="font-mono text-foreground break-all">{uploadToRestore}</span>? All
              current projects, deployments, and configurations will be replaced.
            </DialogDescription>
          </DialogHeader>
          <Callout tone="danger">This will replace all current data.</Callout>
          <Field label="Confirm with account password">
            <Input
              type="password"
              value={restorePassword}
              onChange={(e) => setRestorePassword(e.target.value)}
              placeholder="Your DockLift password"
              autoComplete="current-password"
            />
          </Field>
          <DialogFooter>
            <Button variant="outline" onClick={() => {
              setShowRestoreUploadConfirm(false);
              setUploadToRestore(null);
              setRestorePassword('');
            }}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleRestoreFromUpload}
              disabled={restoringBackup || !restorePassword.trim()}
            >
              {restoringBackup && <Loader2 className="animate-spin" />}
              Restore backup
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Backup Progress Dialog */}
      <Dialog open={showBackupProgress} onOpenChange={(open) => !creatingBackup && setShowBackupProgress(open)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {creatingBackup ? (
                <Loader2 className="size-4 animate-spin text-subtle" />
              ) : (
                <Check className="size-4 text-ok" />
              )}
              {creatingBackup ? 'Creating backup…' : 'Backup complete'}
            </DialogTitle>
          </DialogHeader>
          <LogBox lines={backupProgress} />
          {!creatingBackup && (
            <DialogFooter>
              <Button variant="outline" onClick={() => setShowBackupProgress(false)}>Close</Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

      {/* Restore Progress Dialog */}
      <Dialog open={showRestoreProgress} onOpenChange={(open) => !(restoringBackup || uploadingBackup) && setShowRestoreProgress(open)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {(restoringBackup || uploadingBackup) ? (
                <Loader2 className="size-4 animate-spin text-subtle" />
              ) : (
                <Check className="size-4 text-ok" />
              )}
              {(restoringBackup || uploadingBackup) ? 'Restoring backup…' : 'Restore complete'}
            </DialogTitle>
          </DialogHeader>
          <LogBox lines={restoreProgress} />
          {!(restoringBackup || uploadingBackup) && (
            <DialogFooter>
              <Button variant="outline" onClick={() => setShowRestoreProgress(false)}>Close</Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

    </>
  );
}

export default function SettingsPage() {
  return (
    <Suspense fallback={
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="size-5 animate-spin text-subtle" />
      </div>
    }>
      <SettingsContent />
    </Suspense>
  );
}

/* ---------- presentational helpers (layout only, no state) ---------- */

const TAB_DESCRIPTIONS: Record<string, string> = {
  profile: "Your name, email and sign-in password for this Docklift account.",
  github: "The GitHub App Docklift uses to list and deploy repositories.",
  server: "Addresses and storage path of this Docklift installation.",
  port: "Host ports Docklift assigns to new deployments.",
  docker: "The Docker network application containers join.",
  domain: "Serve the dashboard at a hostname with a Let's Encrypt certificate.",
  backup: "Create, download and delete full backups of this server's Docklift data.",
  restore: "Replace all Docklift data on this server with an uploaded backup.",
};

function Panel({
  title,
  description,
  meta,
  children,
}: {
  title: string;
  description?: string;
  meta?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <Card>
      <div className="border-b border-border px-4 py-3 sm:px-5">
        <h2 className="flex items-baseline gap-2 text-[15px] font-semibold leading-5">
          {title}
          {meta !== undefined && (
            <span className="font-mono text-[13px] font-normal text-subtle tabular-nums">{meta}</span>
          )}
        </h2>
        {description && (
          <p className="mt-0.5 text-[13px] leading-5 text-muted-foreground">{description}</p>
        )}
      </div>
      {children}
    </Card>
  );
}

function PanelBody({ children }: { children: React.ReactNode }) {
  return (
    <div className="p-4 sm:p-5">
      <div className="max-w-xl">{children}</div>
    </div>
  );
}

function PanelFooter({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col-reverse gap-2 border-t border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-end sm:px-5">
      {children}
    </div>
  );
}

function Field({
  label,
  help,
  optional,
  children,
}: {
  label: string;
  help?: React.ReactNode;
  optional?: boolean;
  children: React.ReactNode;
}) {
  return (
    <label className="block space-y-1.5">
      <span className="block text-[13px] font-medium leading-5">
        {label}
        {optional && <span className="ml-1.5 font-normal text-subtle">Optional</span>}
      </span>
      {children}
      {help && <span className="block text-xs leading-[1.125rem] text-muted-foreground">{help}</span>}
    </label>
  );
}

function DefRow({
  label,
  help,
  children,
}: {
  label: string;
  help?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid gap-1 px-4 py-3 sm:grid-cols-[12rem_1fr] sm:gap-4 sm:px-5">
      <dt className="text-[13px] leading-5 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-[13px] leading-5">
        {children}
        {help && <p className="mt-1 text-xs leading-[1.125rem] text-muted-foreground">{help}</p>}
      </dd>
    </div>
  );
}

function CopyRow({
  label,
  value,
  help,
  onCopy,
}: {
  label: string;
  value: string;
  help: string;
  onCopy: () => void;
}) {
  return (
    <div className="grid gap-1 px-4 py-3 sm:grid-cols-[12rem_1fr] sm:gap-4 sm:px-5">
      <dt className="text-[13px] leading-5 text-muted-foreground">{label}</dt>
      <dd className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-mono text-[13px] leading-5 break-all text-foreground">{value}</p>
          <p className="mt-1 text-xs leading-[1.125rem] text-muted-foreground">{help}</p>
        </div>
        <Button variant="ghost" size="icon" className="-my-1.5 shrink-0 text-subtle" onClick={onCopy}>
          <Copy />
          <span className="sr-only">Copy {label}</span>
        </Button>
      </dd>
    </div>
  );
}

function StatusDot({ tone, children }: { tone: "ok" | "none"; children: React.ReactNode }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 text-[13px] font-medium",
        tone === "ok" ? "text-foreground" : "text-muted-foreground",
      )}
    >
      <span
        aria-hidden="true"
        className={cn("size-2 rounded-full", tone === "ok" ? "bg-ok" : "bg-subtle")}
      />
      {children}
    </span>
  );
}

function Callout({
  tone,
  title,
  children,
}: {
  tone: "info" | "warn" | "danger";
  title?: string;
  children: React.ReactNode;
}) {
  const border = { info: "border-info/40", warn: "border-warn/40", danger: "border-danger/40" }[tone];
  return (
    <div className={cn("rounded-md border bg-secondary/60 px-3 py-2.5 text-[13px] leading-5", border)}>
      {title && (
        <p className={cn("font-medium", tone === "danger" ? "text-danger" : "text-foreground")}>{title}</p>
      )}
      <div className={cn(title ? "mt-0.5 text-muted-foreground" : "text-foreground")}>{children}</div>
    </div>
  );
}

function EmptyState({ title, line }: { title: string; line: string }) {
  return (
    <div className="px-4 py-14 text-center">
      <p className="text-[15px] font-semibold">{title}</p>
      <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">{line}</p>
    </div>
  );
}

function FileTable({
  nameHead,
  dateHead,
  rows,
  formatBytes,
  renderTag,
  renderActions,
}: {
  nameHead: string;
  dateHead: string;
  rows: BackupInfo[];
  formatBytes: (bytes: number) => string;
  renderTag?: (row: BackupInfo) => React.ReactNode;
  renderActions: (row: BackupInfo) => React.ReactNode;
}) {
  return (
    <table className="w-full table-fixed text-left text-[13px]">
      <thead>
        <tr className="h-9 text-xs text-muted-foreground">
          <th className="px-4 font-medium sm:pl-5">{nameHead}</th>
          <th className="hidden w-28 px-3 font-medium sm:table-cell">Size</th>
          <th className="hidden w-48 px-3 font-medium md:table-cell">{dateHead}</th>
          <th className="w-28 px-4 text-right font-medium sm:w-36 sm:pr-5">
            <span className="sr-only sm:not-sr-only">Actions</span>
          </th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.filename} className="h-11 border-t border-border transition-colors duration-150 hover:bg-secondary/50">
            <td className="px-4 py-2 sm:pl-5">
              <div className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-0.5">
                <span className="min-w-0 font-mono text-[13px] text-foreground max-sm:break-all sm:truncate" title={row.filename}>
                  {row.filename}
                </span>
                {renderTag?.(row)}
              </div>
              <span className="font-mono text-xs text-muted-foreground sm:hidden">
                {formatBytes(row.size)}
              </span>
            </td>
            <td className="hidden px-3 font-mono text-xs text-muted-foreground tabular-nums sm:table-cell">
              {formatBytes(row.size)}
            </td>
            <td className="hidden px-3 text-[13px] text-muted-foreground md:table-cell">
              {new Date(row.created_at).toLocaleString()}
            </td>
            <td className="px-4 sm:pr-5">
              <div className="flex items-center justify-end gap-1">{renderActions(row)}</div>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function LogBox({ lines }: { lines: string[] }) {
  return (
    <div className="shell-scroll max-h-[400px] min-h-24 overflow-y-auto rounded-lg border border-border bg-[#0f1215] px-4 py-3 font-mono text-xs leading-5 text-white/85">
      {lines.map((line, i) => (
        <div key={i} className="whitespace-pre-wrap">{line}</div>
      ))}
    </div>
  );
}
