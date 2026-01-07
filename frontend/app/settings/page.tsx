// Settings page - GitHub integration, server config, port allocation, Docker, and domain management
"use client";

import { useState, useEffect, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Server, Network, Container, Info, Loader2, Check, X, Sparkles, Globe, Plus, Trash2, ExternalLink, Copy, AlertTriangle, User, Lock, ShieldCheck, KeyRound, Mail, UserCircle, HardDrive, Download, RotateCcw, Archive, Upload, FileUp } from "lucide-react";
import { GithubIcon } from "@/components/icons/GithubIcon";
import { toast } from "sonner";
import { API_URL, copyToClipboard } from "@/lib/utils";
import { GitHubConnect } from "@/components/GitHubConnect";
import { getAuthHeaders } from "@/lib/auth";
import { useAuth } from "@/components/AuthProvider";

interface GitHubStatus {
  connected: boolean;
  username?: string;
  avatar_url?: string;
  name?: string;
  error?: string;
  installUrl?: string; // URL to configure/install app on more accounts
}

interface DomainConfig {
  domain: string;
  port: number;
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
  const [installUrl, setInstallUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [disconnecting, setDisconnecting] = useState(false);
  const [showGitHubConnect, setShowGitHubConnect] = useState(false);
  const [activeTab, setActiveTab] = useState('profile');
  
  // Profile State
  const { user, updateUser } = useAuth();
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
  const [showAddDomain, setShowAddDomain] = useState(false);
  const [newDomain, setNewDomain] = useState({ domain: '', port: '8080' });
  const [addingDomain, setAddingDomain] = useState(false);
  const [serverIP, setServerIP] = useState<string>('...');
  
  // Delete Confirmation State
  const [isDeleteConfirmOpen, setIsDeleteConfirmOpen] = useState(false);
  const [domainToDelete, setDomainToDelete] = useState<string | null>(null);
  const [deletingDomain, setDeletingDomain] = useState(false);

  // Backup State
  const [backups, setBackups] = useState<BackupInfo[]>([]);
  const [loadingBackups, setLoadingBackups] = useState(false);
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

  const searchParams = useSearchParams();
  const router = useRouter();

  // Sync activeTab with URL query parameter
  useEffect(() => {
    const tab = searchParams.get("tab");
    const validTabs = ['github', 'server', 'port', 'docker', 'domain', 'profile', 'backup', 'restore'];

    if (tab && validTabs.includes(tab)) {
      setActiveTab(tab);
    } else if (!tab) {
      setActiveTab('profile');
    }

    if (searchParams.get("github") === "connected") {
      toast.success("GitHub account connected successfully!");
    }
  }, [searchParams]);

  // Fetch data based on active tab
  useEffect(() => {
    if (activeTab === 'github') fetchGitHubStatus();
    if (activeTab === 'domain') fetchDomains();
    if (activeTab === 'server') fetchServerIP();
    if (activeTab === 'backup' || activeTab === 'restore') fetchBackups();
  }, [activeTab]);

  useEffect(() => {
    if (user) {
      setProfileData({ name: user.name || '', email: user.email || '' });
    }
  }, [user]);

  const handleTabChange = (tabId: string) => {
    router.push(`/settings?tab=${tabId}`, { scroll: false });
  };

  const fetchServerIP = async () => {
    try {
      const res = await fetch(`${API_URL}/api/system/ip`, { headers: getAuthHeaders() });
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
      
      const res = await fetch(`${API_URL}/api/github/status`, {
        signal: controller.signal,
        headers: getAuthHeaders()
      });
      clearTimeout(timeoutId);
      const data = await res.json();
      setGithubStatus(data);

      // Also fetch all installations if connected
      if (data.connected) {
        try {
          const instRes = await fetch(`${API_URL}/api/github/installations`, { headers: getAuthHeaders() });
          if (instRes.ok) {
            const instData = await instRes.json();
            setGithubInstallations(instData.installations || []);
            setInstallUrl(instData.installUrl || null);
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

  const handleDisconnectGitHub = async () => {
    setDisconnecting(true);
    try {
      await fetch(`${API_URL}/api/github/disconnect`, { method: "POST", headers: getAuthHeaders() });
      setGithubStatus({ connected: false });
      toast.success("GitHub disconnected");
    } catch {
      toast.error("Failed to disconnect");
    } finally {
      setDisconnecting(false);
    }
  };

  const fetchDomains = async () => {
    setLoadingDomains(true);
    try {
      const res = await fetch(`${API_URL}/api/domains`, { headers: getAuthHeaders() });
      if (res.ok) {
        const data = await res.json();
        setDomains(data);
      }
    } catch (error) {
      toast.error("Failed to load domains");
    } finally {
      setLoadingDomains(false);
    }
  };

  const handleAddDomain = async () => {
    if (!newDomain.domain) {
      toast.error("Please enter a domain name");
      return;
    }

    const port = newDomain.port || '8080';

    setAddingDomain(true);
    try {
      const res = await fetch(`${API_URL}/api/domains`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...getAuthHeaders() },
        body: JSON.stringify({
          domain: newDomain.domain,
          port: parseInt(port)
        })
      });

      const data = await res.json();
      
      if (!res.ok) throw new Error(data.error || "Failed to add domain");
      
      toast.success(`Domain ${newDomain.domain} added successfully`);
      setShowAddDomain(false);
      setNewDomain({ domain: '', port: '8080' });
      fetchDomains();
    } catch (error: any) {
      toast.error(error.message);
    } finally {
      setAddingDomain(false);
    }
  };

  const handleDeleteDomain = (domain: string) => {
    setDomainToDelete(domain);
    setIsDeleteConfirmOpen(true);
  };

  const confirmDeleteDomain = async () => {
    if (!domainToDelete) return;

    setDeletingDomain(true);
    try {
      const res = await fetch(`${API_URL}/api/domains/${domainToDelete}`, {
        method: "DELETE",
        headers: getAuthHeaders()
      });
      
      if (!res.ok) throw new Error("Failed to delete domain");
      
      toast.success("Domain removed");
      setIsDeleteConfirmOpen(false);
      setDomainToDelete(null);
      fetchDomains();
    } finally {
      setDeletingDomain(false);
    }
  };

  const handleUpdateProfile = async (e: React.FormEvent) => {
    e.preventDefault();
    setUpdatingProfile(true);
    try {
      const res = await fetch(`${API_URL}/api/auth/profile`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", ...getAuthHeaders() },
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
      const res = await fetch(`${API_URL}/api/auth/change-password`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...getAuthHeaders() },
        body: JSON.stringify({
          currentPassword: passwordData.currentPassword,
          newPassword: passwordData.newPassword
        })
      });

      const data = await res.json();
      if (!res.ok) throw new Error(data.error || "Failed to change password");
      
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
      const res = await fetch(`${API_URL}/api/backup`, { headers: getAuthHeaders() });
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

  const handleCreateBackup = async () => {
    setCreatingBackup(true);
    setBackupProgress([]);
    setShowBackupProgress(true);

    try {
      const res = await fetch(`${API_URL}/api/backup/create`, {
        method: 'POST',
        headers: getAuthHeaders(),
      });

      const reader = res.body?.getReader();
      const decoder = new TextDecoder();

      if (reader) {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          const text = decoder.decode(value);
          const lines = text.split('\n').filter(line => line.trim());
          setBackupProgress(prev => [...prev, ...lines]);
        }
      }

      toast.success("Backup created successfully");
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
      const res = await fetch(`${API_URL}/api/backup/${backupToDelete}`, {
        method: 'DELETE',
        headers: getAuthHeaders(),
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

  const handleDownloadBackup = (filename: string) => {
    // Open download in new tab with auth
    const token = localStorage.getItem('docklift_token');
    window.open(`${API_URL}/api/backup/download/${filename}?token=${token}`, '_blank');
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

    setUploadingBackup(true);
    setRestoreProgress([]);
    setShowRestoreProgress(true);
    setShowUploadRestoreConfirm(false);

    try {
      const formData = new FormData();
      formData.append('backup', selectedUploadFile);

      // Get auth token but don't set Content-Type (let browser set it for multipart/form-data)
      const token = localStorage.getItem('docklift_token');
      const headers: HeadersInit = {};
      if (token) {
        headers['Authorization'] = `Bearer ${token}`;
      }

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

      toast.success("Backup restored successfully");
      fetchBackups();
    } catch (error) {
      toast.error("Failed to restore from uploaded backup");
    } finally {
      setUploadingBackup(false);
      setSelectedUploadFile(null);
    }
  };

  return (
    <div className="min-h-screen flex flex-col bg-background">
      <Header />

      <main className="flex-1 container max-w-6xl mx-auto px-4 md:px-6 py-8">
        <div className="mb-8">
          <h1 className="text-3xl font-bold tracking-tight">Settings</h1>
          <p className="text-muted-foreground mt-1">Manage your server, integrations, and domains</p>
        </div>

        <div className="flex flex-col md:flex-row gap-8">
          {/* Sidebar Navigation */}
          <aside className="w-full md:w-64 shrink-0 space-y-2">
            <nav className="flex md:flex-col overflow-x-auto md:overflow-visible gap-2 p-1">
              {[
                { id: 'profile', label: 'Profile', icon: User },
                { id: 'github', label: 'GitHub', icon: GithubIcon },
                { id: 'server', label: 'Server', icon: Server },
                { id: 'port', label: 'Port', icon: Network },
                { id: 'docker', label: 'Docker', icon: Container },
                { id: 'domain', label: 'Domains', icon: Globe },
                { id: 'backup', label: 'Backup', icon: Archive },
                { id: 'restore', label: 'Restore', icon: RotateCcw },
              ].map((item) => {
                const Icon = item.icon;
                const isActive = activeTab === item.id;
                return (
                  <button
                    key={item.id}
                    onClick={() => handleTabChange(item.id)}
                    className={`flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-medium transition-all whitespace-nowrap
                      ${isActive 
                        ? "bg-primary text-primary-foreground shadow-lg shadow-primary/20 scale-[1.02]" 
                        : "hover:bg-muted text-muted-foreground hover:text-foreground"
                      }`}
                  >
                    <Icon className={`h-4 w-4 ${isActive ? "text-primary-foreground" : ""}`} />
                    {item.label}
                  </button>
                );
              })}
            </nav>
          </aside>

          {/* Main Content Area */}
          <div className="flex-1 min-w-0">
            {/* Profile Tab */}
            {activeTab === 'profile' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <div className="grid gap-6">
                  {/* Account Info */}
                  <Card className="p-6 border-cyan-500/10 bg-card/50 backdrop-blur-sm">
                    <div className="flex items-center gap-4 mb-6">
                      <div className="p-3 rounded-2xl bg-cyan-500/10 border border-cyan-500/20">
                        <UserCircle className="h-6 w-6 text-cyan-500" />
                      </div>
                      <div>
                        <h2 className="text-xl font-black">Account Profile</h2>
                        <p className="text-xs text-muted-foreground uppercase tracking-widest font-bold mt-0.5">Personal Identity</p>
                      </div>
                    </div>
                    
                    <form onSubmit={handleUpdateProfile} className="space-y-4">
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                        <div className="space-y-2">
                          <label className="text-xs font-black uppercase tracking-wider text-muted-foreground ml-1">Full Name</label>
                          <Input 
                            value={profileData.name}
                            onChange={(e) => setProfileData({ ...profileData, name: e.target.value })}
                            placeholder="John Doe"
                            className="bg-secondary/30 h-11 border-border/40 focus:border-cyan-500/50 transition-all font-medium"
                          />
                        </div>
                        <div className="space-y-2">
                          <label className="text-xs font-black uppercase tracking-wider text-muted-foreground ml-1">Email Address</label>
                          <div className="relative">
                            <Mail className="absolute left-3.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground/50" />
                            <Input 
                              type="email"
                              value={profileData.email}
                              onChange={(e) => setProfileData({ ...profileData, email: e.target.value })}
                              placeholder="john@example.com"
                              className="bg-secondary/30 h-11 pl-10 border-border/40 focus:border-cyan-500/50 transition-all font-medium"
                            />
                          </div>
                        </div>
                      </div>
                      <div className="flex justify-end pt-2">
                        <Button 
                          type="submit" 
                          disabled={updatingProfile}
                          className="bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white font-black px-6 rounded-xl shadow-lg shadow-cyan-500/20"
                        >
                          {updatingProfile ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Check className="h-4 w-4 mr-2" />}
                          Update Profile
                        </Button>
                      </div>
                    </form>
                  </Card>

                  {/* Security Section */}
                  <Card className="p-6 border-red-500/10 bg-card/50 backdrop-blur-sm">
                    <div className="flex items-center gap-4 mb-6">
                      <div className="p-3 rounded-2xl bg-red-500/10 border border-red-500/20">
                        <ShieldCheck className="h-6 w-6 text-red-500" />
                      </div>
                      <div>
                        <h2 className="text-xl font-black">Security</h2>
                        <p className="text-xs text-muted-foreground uppercase tracking-widest font-bold mt-0.5">Password Management</p>
                      </div>
                    </div>

                    <form onSubmit={handleChangePassword} className="space-y-4">
                      <div className="space-y-2">
                        <label className="text-xs font-black uppercase tracking-wider text-muted-foreground ml-1">Current Password</label>
                        <div className="relative">
                          <Lock className="absolute left-3.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground/50" />
                          <Input 
                            type={showCurrentPassword ? "text" : "password"}
                            value={passwordData.currentPassword}
                            onChange={(e) => setPasswordData({ ...passwordData, currentPassword: e.target.value })}
                            placeholder="Current Password"
                            className="bg-secondary/30 h-11 pl-10 pr-10 border-border/40 focus:border-red-500/30 transition-all font-medium"
                          />
                          <button 
                            type="button" 
                            onClick={() => setShowCurrentPassword(!showCurrentPassword)}
                            className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                          >
                            {showCurrentPassword ? <Lock className="h-4 w-4" /> : <ShieldCheck className="h-4 w-4" />}
                          </button>
                        </div>
                      </div>
                      
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                        <div className="space-y-2">
                          <label className="text-xs font-black uppercase tracking-wider text-muted-foreground ml-1">New Password</label>
                          <div className="relative">
                            <KeyRound className="absolute left-3.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground/50" />
                            <Input 
                              type={showNewPassword ? "text" : "password"}
                              value={passwordData.newPassword}
                              onChange={(e) => setPasswordData({ ...passwordData, newPassword: e.target.value })}
                              placeholder="New Password"
                              className="bg-secondary/30 h-11 pl-10 pr-10 border-border/40 focus:border-cyan-500/30 transition-all font-medium"
                            />
                            <button 
                              type="button" 
                              onClick={() => setShowNewPassword(!showNewPassword)}
                              className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                            >
                              {showNewPassword ? <Lock className="h-4 w-4" /> : <ShieldCheck className="h-4 w-4" />}
                            </button>
                          </div>
                        </div>
                        <div className="space-y-2">
                          <label className="text-xs font-black uppercase tracking-wider text-muted-foreground ml-1">Confirm New Password</label>
                          <div className="relative">
                            <KeyRound className="absolute left-3.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground/50" />
                            <Input 
                              type="password"
                              value={passwordData.confirmPassword}
                              onChange={(e) => setPasswordData({ ...passwordData, confirmPassword: e.target.value })}
                              placeholder="Confirm Password"
                              className="bg-secondary/30 h-11 pl-10 border-border/40 focus:border-cyan-500/30 transition-all font-medium"
                            />
                          </div>
                        </div>
                      </div>

                      <div className="flex justify-end pt-2">
                        <Button 
                          type="submit" 
                          disabled={updatingPassword || !passwordData.newPassword}
                          className="bg-gradient-to-r from-violet-600 to-indigo-600 hover:from-violet-700 hover:to-indigo-700 text-white font-black px-6 rounded-xl shadow-lg shadow-violet-500/20 active:scale-95 transition-all border-none"
                        >
                          {updatingPassword ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <ShieldCheck className="h-4 w-4 mr-2" />}
                          Change Password
                        </Button>
                      </div>
                    </form>
                  </Card>
                </div>
              </div>
            )}

            {/* GitHub Tab */}
            {activeTab === 'github' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <div className="relative group">
                  <div className="absolute inset-0 bg-gradient-to-r from-cyan-500/20 to-blue-500/20 rounded-3xl blur-xl opacity-75" />
                  <Card className="relative p-6 border-cyan-500/20 bg-card/50 backdrop-blur-sm">
                    <div className="flex flex-col sm:flex-row items-center gap-6">
                      <div className="relative shrink-0">
                        <div className="absolute inset-0 bg-cyan-500/20 blur-xl rounded-full" />
                        <div className="relative p-4 rounded-2xl bg-gradient-to-br from-zinc-900 to-black border border-zinc-800 shadow-xl">
                          <GithubIcon className="h-8 w-8 text-white" />
                        </div>
                      </div>
                      
                      <div className="flex-1 text-center sm:text-left space-y-1">
                        <h2 className="text-xl font-bold">GitHub Integration</h2>
                        <p className="text-muted-foreground">
                          Connect to automatically deploy public and private repositories.
                        </p>
                      </div>

                      <div className="shrink-0 pt-4 sm:pt-0">
                        {loading ? (
                          <div className="flex items-center gap-2 text-muted-foreground px-4 py-2 bg-secondary/50 rounded-lg">
                            <Loader2 className="h-4 w-4 animate-spin" />
                            <span>Checking...</span>
                          </div>
                        ) : githubStatus?.connected ? (
                          <div className="flex flex-col items-center sm:items-end gap-3 w-full sm:w-auto">
                            {/* Multi-account list */}
                            <div className="flex flex-col gap-2 w-full sm:w-auto">
                              {githubInstallations.length > 0 ? (
                                githubInstallations.map((inst) => (
                                  <div 
                                    key={inst.id} 
                                    className="flex items-center gap-3 bg-green-500/10 border border-green-500/20 px-4 py-2.5 rounded-xl"
                                  >
                                    {inst.avatar_url ? (
                                      <img src={inst.avatar_url} alt={inst.login} className="h-8 w-8 rounded-lg border border-green-500/20" />
                                    ) : (
                                      <div className="h-8 w-8 rounded-lg bg-green-500/20 flex items-center justify-center font-bold text-green-500 text-sm">
                                        {inst.login?.charAt(0).toUpperCase()}
                                      </div>
                                    )}
                                    <div className="flex flex-col">
                                      <span className="text-sm font-medium">@{inst.login}</span>
                                      <span className="text-xs text-muted-foreground">
                                        {inst.type === 'Organization' ? '🏢 Organization' : '👤 Personal'}
                                      </span>
                                    </div>
                                    <Check className="h-4 w-4 text-green-500 ml-auto" />
                                  </div>
                                ))
                              ) : (
                                <div className="flex items-center gap-3 bg-green-500/10 border border-green-500/20 px-4 py-2.5 rounded-xl">
                                  <Check className="h-5 w-5 text-green-500" />
                                  <span className="text-sm font-medium text-green-500">Connected</span>
                                </div>
                              )}
                            </div>
                            
                            <div className="flex items-center gap-2">
                              {installUrl && (
                                <a 
                                  href={installUrl}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  title="Add another account or organization — repos will be combined"
                                >
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    className="h-9 px-3 rounded-xl border-cyan-500/20 text-cyan-500 hover:text-cyan-600 hover:bg-cyan-500/10"
                                  >
                                    <Plus className="h-4 w-4 mr-1.5" />
                                    Add Account
                                  </Button>
                                </a>
                              )}
                              <Button 
                                variant="destructive" 
                                size="sm"
                                onClick={handleDisconnectGitHub}
                                disabled={disconnecting}
                                className="h-9 px-4 text-xs font-semibold shadow-lg shadow-red-500/20 hover:bg-red-600 transition-all rounded-xl"
                              >
                                {disconnecting ? (
                                  <Loader2 className="h-3.5 w-3.5 animate-spin mr-2" />
                                ) : (
                                  <X className="h-3.5 w-3.5 mr-2" />
                                )}
                                Disconnect
                              </Button>
                            </div>
                          </div>
                        ) : (
                          <Button 
                            onClick={handleConnectGitHub}
                            size="lg"
                            className="gap-2 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white shadow-lg shadow-cyan-500/25"
                          >
                            <Sparkles className="h-4 w-4" />
                            Connect GitHub
                          </Button>
                        )}
                        <GitHubConnect 
                          open={showGitHubConnect} 
                          onOpenChange={setShowGitHubConnect}
                          onConnected={fetchGitHubStatus}
                        />
                      </div>
                    </div>
                  </Card>
                </div>
              </div>
            )}

            {/* Server Tab */}
            {activeTab === 'server' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <Card className="p-6">
                  <div className="flex items-center gap-3 mb-6">
                    <div className="p-3 rounded-xl bg-primary/10">
                      <Server className="h-6 w-6 text-primary" />
                    </div>
                    <div>
                      <h2 className="text-xl font-semibold">Server Configuration</h2>
                      <p className="text-sm text-muted-foreground">Core system and API settings</p>
                    </div>
                  </div>
                  <div className="grid gap-6">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Server IP Address</label>
                      <div className="flex gap-2">
                        <Input 
                          value={serverIP} 
                          disabled 
                          className="bg-secondary/50 font-mono" 
                        />
                        <Button
                          variant="outline"
                          size="icon"
                          onClick={() => {
                            copyToClipboard(serverIP);
                            toast.success('IP copied!');
                          }}
                        >
                          <Copy className="h-4 w-4" />
                        </Button>
                      </div>
                      <p className="text-xs text-muted-foreground">Your server&apos;s public IP address.</p>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Docklift Panel URL</label>
                      <div className="flex gap-2">
                        <Input 
                          value={`http://${serverIP}:8080`} 
                          disabled 
                          className="bg-secondary/50 font-mono" 
                        />
                        <Button
                          variant="outline"
                          size="icon"
                          onClick={() => {
                            copyToClipboard(`http://${serverIP}:8080`);
                            toast.success('URL copied!');
                          }}
                        >
                          <Copy className="h-4 w-4" />
                        </Button>
                      </div>
                      <p className="text-xs text-muted-foreground">Access Docklift at this URL (Frontend port 8080).</p>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Backend API URL</label>
                      <div className="flex gap-2">
                        <Input 
                          value={`http://${serverIP}:4000`} 
                          disabled 
                          className="bg-secondary/50 font-mono" 
                        />
                        <Button
                          variant="outline"
                          size="icon"
                          onClick={() => {
                            copyToClipboard(`http://${serverIP}:4000`);
                            toast.success('URL copied!');
                          }}
                        >
                          <Copy className="h-4 w-4" />
                        </Button>
                      </div>
                      <p className="text-xs text-muted-foreground">Backend API endpoint (Port 4000).</p>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Deployments Directory</label>
                      <div className="flex gap-2">
                        <Input value="/deployments" disabled className="bg-secondary/50 font-mono" />
                        <Button
                          variant="outline"
                          size="icon"
                          onClick={() => {
                            copyToClipboard('/deployments');
                            toast.success('Path copied!');
                          }}
                        >
                          <Copy className="h-4 w-4" />
                        </Button>
                      </div>
                      <p className="text-xs text-muted-foreground">Absolute path where project files are stored on the host.</p>
                    </div>
                  </div>
                </Card>
              </div>
            )}

            {/* Port Tab */}
            {activeTab === 'port' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <Card className="p-6">
                  <div className="flex items-center gap-3 mb-6">
                    <div className="p-3 rounded-xl bg-emerald-500/10">
                      <Network className="h-6 w-6 text-emerald-500" />
                    </div>
                    <div>
                      <h2 className="text-xl font-semibold">Port Allocation</h2>
                      <p className="text-sm text-muted-foreground">Manage the range of ports available for deployments</p>
                    </div>
                  </div>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Start Port</label>
                      <Input type="number" value="3001" disabled className="bg-secondary/50 font-mono" />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">End Port</label>
                      <Input type="number" value="3100" disabled className="bg-secondary/50 font-mono" />
                    </div>
                  </div>
                  <div className="mt-4 p-4 bg-secondary/30 rounded-lg text-sm text-muted-foreground">
                    <p>Docklift automatically assigns the next available port from this pool when creating new deployments.</p>
                  </div>
                </Card>
              </div>
            )}

            {/* Docker Tab */}
            {activeTab === 'docker' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <Card className="p-6">
                  <div className="flex items-center gap-3 mb-6">
                    <div className="p-3 rounded-xl bg-blue-500/10">
                      <Container className="h-6 w-6 text-blue-500" />
                    </div>
                    <div>
                      <h2 className="text-xl font-semibold">Docker Network</h2>
                      <p className="text-sm text-muted-foreground">Container orchestration settings</p>
                    </div>
                  </div>
                  <div className="space-y-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Network Bridge</label>
                      <Input value="docklift_network" disabled className="bg-secondary/50 font-mono" />
                      <p className="text-xs text-muted-foreground">
                        All application containers are attached to this bridge network to allow internal communication.
                      </p>
                    </div>
                  </div>
                </Card>
              </div>
            )}

            {/* Domain Tab */}
            {activeTab === 'domain' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <Card className="p-4 sm:p-6 border-indigo-500/20">
                  <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
                    <div className="flex items-center gap-3">
                      <div className="p-2 sm:p-3 rounded-xl bg-indigo-500/10 shrink-0">
                        <Globe className="h-5 w-5 sm:h-6 sm:w-6 text-indigo-500" />
                      </div>
                      <div className="min-w-0">
                        <h2 className="text-lg sm:text-xl font-semibold">Server Domain</h2>
                        <p className="text-xs sm:text-sm text-muted-foreground">Access your Docklift panel via custom domain instead of IP</p>
                      </div>
                    </div>
                    
                    <Dialog open={showAddDomain} onOpenChange={setShowAddDomain}>
                      <DialogTrigger asChild>
                        <Button className="bg-indigo-600 hover:bg-indigo-700 w-full sm:w-auto shrink-0">
                          <Plus className="h-4 w-4 mr-2" />
                          Add Domain
                        </Button>
                      </DialogTrigger>
                      <DialogContent>
                        <DialogHeader>
                          <DialogTitle>Add Server Domain</DialogTitle>
                          <DialogDescription>
                            Access Docklift using a custom domain instead of IP address. Configure your DNS first!
                          </DialogDescription>
                        </DialogHeader>
                        
                        {/* DNS Instructions */}
                        <div className="p-3 rounded-lg bg-amber-500/10 border border-amber-500/20 text-sm space-y-2">
                          <p className="font-medium text-amber-600 dark:text-amber-400">📝 DNS Setup Required</p>
                          <p className="text-muted-foreground">Add an <strong>A Record</strong> in your domain's DNS settings:</p>
                          <div className="font-mono text-xs bg-secondary/50 p-2 rounded">
                            <div className="grid grid-cols-3 gap-2">
                              <span className="text-muted-foreground">Type</span>
                              <span className="text-muted-foreground">Host</span>
                              <span className="text-muted-foreground">Value</span>
                            </div>
                            <div className="grid grid-cols-3 gap-2 font-bold">
                              <span>A</span>
                              <span>@ or subdomain</span>
                              <span>Your Server IP</span>
                            </div>
                          </div>
                          <p className="text-xs text-muted-foreground">Use <code className="bg-secondary px-1 rounded">@</code> for root domain (example.com) or a subdomain like <code className="bg-secondary px-1 rounded">panel</code> for panel.example.com</p>
                        </div>
                        <div className="space-y-4 py-4">
                          <div className="space-y-2">
                            <label className="text-sm font-medium">Domain Name</label>
                            <Input 
                              placeholder="panel.yourdomain.com" 
                              value={newDomain.domain}
                              onChange={(e) => setNewDomain({ ...newDomain, domain: e.target.value })}
                            />
                          </div>
                          <div className="space-y-2">
                            <label className="text-sm font-medium">Docklift Port</label>
                            <Input 
                              type="number" 
                              placeholder="8080" 
                              value={newDomain.port}
                              onChange={(e) => setNewDomain({ ...newDomain, port: e.target.value })}
                            />
                            <p className="text-xs text-muted-foreground">Docklift runs on port 8080 by default. Change only if you modified it.</p>
                          </div>
                        </div>
                        <DialogFooter>
                          <Button variant="outline" onClick={() => setShowAddDomain(false)}>Cancel</Button>
                          <Button onClick={handleAddDomain} disabled={addingDomain} className="bg-indigo-600 hover:bg-indigo-700">
                            {addingDomain && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                            Add Mapping
                          </Button>
                        </DialogFooter>
                      </DialogContent>
                    </Dialog>
                  </div>

                  {loadingDomains ? (
                    <div className="flex justify-center p-8">
                      <Loader2 className="h-8 w-8 animate-spin text-indigo-500" />
                    </div>
                  ) : domains.length > 0 ? (
                    <div className="rounded-xl border border-border/50 overflow-hidden">
                      <table className="w-full text-left text-sm">
                        <thead className="bg-secondary/50 font-medium text-muted-foreground">
                          <tr>
                            <th className="px-4 py-3">Domain</th>
                            <th className="px-4 py-3">Target Port</th>
                            <th className="px-4 py-3 text-right">Actions</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-border/50">
                          {domains.map((d) => (
                            <tr key={d.domain} className="hover:bg-secondary/20">
                              <td className="px-4 py-3 font-medium flex items-center gap-2">
                                <a href={`http://${d.domain}`} target="_blank" rel="noreferrer" className="flex items-center gap-2 hover:text-indigo-500 hover:underline">
                                  {d.domain}
                                  <ExternalLink className="h-3 w-3 opacity-50" />
                                </a>
                              </td>
                              <td className="px-4 py-3 font-mono text-muted-foreground">{d.port}</td>
                              <td className="px-4 py-3 text-right">
                                <Button 
                                  variant="ghost" 
                                  size="sm" 
                                  className="text-red-500 hover:text-red-600 hover:bg-red-500/10"
                                  onClick={() => handleDeleteDomain(d.domain)}
                                >
                                  <Trash2 className="h-4 w-4" />
                                </Button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    /* Empty State */
                    <div className="rounded-xl border border-border/50 overflow-hidden">
                      <div className="p-8 text-center bg-secondary/20">
                        <div className="inline-flex items-center justify-center p-4 rounded-full bg-secondary/50 mb-4">
                          <Globe className="h-8 w-8 text-muted-foreground/50" />
                        </div>
                        <h3 className="font-medium text-lg mb-1">No Server Domain Configured</h3>
                        <p className="text-muted-foreground text-sm max-w-sm mx-auto mb-4">
                          Access your Docklift panel using a custom domain like <strong>panel.yourdomain.com</strong> instead of IP:Port.
                        </p>
                      </div>
                    </div>
                  )}
                </Card>
              </div>
            )}

            {/* Backup Tab */}
            {activeTab === 'backup' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <Card className="p-4 sm:p-6 border-emerald-500/20">
                  <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
                    <div className="flex items-center gap-3">
                      <div className="p-2 sm:p-3 rounded-xl bg-emerald-500/10 shrink-0">
                        <Archive className="h-5 w-5 sm:h-6 sm:w-6 text-emerald-500" />
                      </div>
                      <div className="min-w-0">
                        <h2 className="text-lg sm:text-xl font-semibold">Create Backup</h2>
                        <p className="text-xs sm:text-sm text-muted-foreground">Create a full system backup with all your data</p>
                      </div>
                    </div>

                    <Button
                      onClick={handleCreateBackup}
                      disabled={creatingBackup}
                      className="bg-emerald-600 hover:bg-emerald-700 w-full sm:w-auto shrink-0"
                    >
                      {creatingBackup ? (
                        <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      ) : (
                        <Archive className="h-4 w-4 mr-2" />
                      )}
                      Create Backup
                    </Button>
                  </div>

                  {/* Backup includes info */}
                  <div className="p-3 rounded-lg bg-secondary/30 border border-border/50 text-sm">
                    <p className="font-medium mb-2">Backups include:</p>
                    <ul className="list-disc list-inside text-muted-foreground space-y-1">
                      <li>Database (projects, deployments, users, settings, env vars)</li>
                      <li>All project files from /deployments/</li>
                      <li>Nginx configurations</li>
                      <li>GitHub App key (if configured)</li>
                    </ul>
                  </div>

                  <div className="mt-4 p-3 rounded-lg bg-amber-500/10 border border-amber-500/20 text-sm">
                    <p className="font-medium text-amber-500 mb-1">Migration Tip</p>
                    <p className="text-muted-foreground text-xs">
                      If you use a <span className="text-foreground font-medium">domain name</span> (e.g., docklift.yourdomain.com)
                      instead of IP address for your GitHub App webhook URL, migrating to a new server becomes seamless -
                      just update DNS and restore. No need to reconfigure the GitHub App.
                    </p>
                  </div>
                </Card>

                {/* Server Backups List */}
                <Card className="p-4 sm:p-6 border-emerald-500/10">
                  <div className="flex items-center gap-3 mb-4">
                    <HardDrive className="h-5 w-5 text-emerald-500" />
                    <h3 className="font-semibold">Server Backups</h3>
                  </div>

                  {loadingBackups ? (
                    <div className="flex justify-center p-8">
                      <Loader2 className="h-8 w-8 animate-spin text-emerald-500" />
                    </div>
                  ) : backups.length > 0 ? (
                    <div className="rounded-xl border border-border/50 overflow-hidden">
                      <table className="w-full text-left text-sm">
                        <thead className="bg-secondary/50 font-medium text-muted-foreground">
                          <tr>
                            <th className="px-4 py-3">Backup</th>
                            <th className="px-4 py-3 hidden sm:table-cell">Size</th>
                            <th className="px-4 py-3 hidden md:table-cell">Created</th>
                            <th className="px-4 py-3 text-right">Actions</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-border/50">
                          {backups.map((backup) => (
                            <tr key={backup.filename} className="hover:bg-secondary/20">
                              <td className="px-4 py-3">
                                <span className="font-medium font-mono text-xs block truncate max-w-[200px]">
                                  {backup.filename}
                                </span>
                                <span className="text-xs text-muted-foreground sm:hidden">
                                  {formatBytes(backup.size)}
                                </span>
                              </td>
                              <td className="px-4 py-3 text-muted-foreground hidden sm:table-cell">
                                {formatBytes(backup.size)}
                              </td>
                              <td className="px-4 py-3 text-muted-foreground hidden md:table-cell">
                                {new Date(backup.created_at).toLocaleString()}
                              </td>
                              <td className="px-4 py-3 text-right space-x-1">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-emerald-500 hover:text-emerald-600 hover:bg-emerald-500/10"
                                  onClick={() => handleDownloadBackup(backup.filename)}
                                  title="Download"
                                >
                                  <Download className="h-4 w-4" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-red-500 hover:text-red-600 hover:bg-red-500/10"
                                  onClick={() => {
                                    setBackupToDelete(backup.filename);
                                    setShowDeleteBackupConfirm(true);
                                  }}
                                  title="Delete"
                                >
                                  <Trash2 className="h-4 w-4" />
                                </Button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <div className="rounded-xl border border-border/50 overflow-hidden">
                      <div className="p-6 text-center bg-secondary/20">
                        <Archive className="h-8 w-8 text-muted-foreground/50 mx-auto mb-2" />
                        <p className="text-muted-foreground text-sm">No backups yet. Create your first backup above.</p>
                      </div>
                    </div>
                  )}
                </Card>
              </div>
            )}

            {/* Restore Tab */}
            {activeTab === 'restore' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                {/* Warning Banner */}
                <div className="p-4 rounded-xl bg-red-500/10 border border-red-500/20">
                  <div className="flex items-start gap-3">
                    <AlertTriangle className="h-5 w-5 text-red-500 mt-0.5 shrink-0" />
                    <div>
                      <p className="font-semibold text-red-500">Warning: Restore replaces all data</p>
                      <p className="text-sm text-muted-foreground mt-1">Restoring from a backup will replace all current projects, deployments, settings, users, and environment variables.</p>
                    </div>
                  </div>
                </div>

                {/* Upload and Restore */}
                <Card className="p-4 sm:p-6 border-amber-500/20">
                  <div className="flex items-center gap-3 mb-6">
                    <div className="p-2 sm:p-3 rounded-xl bg-amber-500/10 shrink-0">
                      <Upload className="h-5 w-5 sm:h-6 sm:w-6 text-amber-500" />
                    </div>
                    <div className="min-w-0">
                      <h2 className="text-lg sm:text-xl font-semibold">Restore from File</h2>
                      <p className="text-xs sm:text-sm text-muted-foreground">Upload a backup file from your computer</p>
                    </div>
                  </div>

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
                          onChange={handleUploadFileSelect}
                          className="hidden"
                          disabled={uploadingBackup || restoringBackup}
                        />
                        <Button
                          type="button"
                          className="bg-amber-600 hover:bg-amber-700"
                          disabled={uploadingBackup || restoringBackup}
                          asChild
                        >
                          <span>
                            {uploadingBackup ? (
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
                    <p className="font-medium mb-2">After restore:</p>
                    <ul className="list-disc list-inside text-muted-foreground space-y-1 text-xs">
                      <li>Sign in with credentials from the backup</li>
                      <li>Redeploy each project (containers need rebuilding)</li>
                      <li>Update DNS if server IP changed</li>
                      <li>GitHub App works automatically if using domain-based webhook URL</li>
                    </ul>
                  </div>
                </Card>

              </div>
            )}

          </div>
        </div>
      </main>

      <Footer />

      {/* Delete Domain Confirmation Dialog */}
      <Dialog open={isDeleteConfirmOpen} onOpenChange={setIsDeleteConfirmOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-red-500">
              <AlertTriangle className="h-5 w-5" />
              Remove Server Domain
            </DialogTitle>
            <DialogDescription className="pt-2">
              Are you sure you want to remove <span className="font-mono font-bold text-foreground">{domainToDelete}</span>? 
              This will stop access to the Docklift panel through this domain.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setIsDeleteConfirmOpen(false)}>
              Cancel
            </Button>
            <Button 
              variant="destructive"
              onClick={confirmDeleteDomain}
              disabled={deletingDomain}
            >
              {deletingDomain ? (
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
              ) : (
                <Trash2 className="h-4 w-4 mr-2" />
              )}
              Remove Domain
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Backup Confirmation Dialog */}
      <Dialog open={showDeleteBackupConfirm} onOpenChange={setShowDeleteBackupConfirm}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-red-500">
              <AlertTriangle className="h-5 w-5" />
              Delete Backup
            </DialogTitle>
            <DialogDescription className="pt-2">
              Are you sure you want to delete <span className="font-mono font-bold text-foreground">{backupToDelete}</span>?
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setShowDeleteBackupConfirm(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteBackup}
              disabled={deletingBackup}
            >
              {deletingBackup ? (
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
              ) : (
                <Trash2 className="h-4 w-4 mr-2" />
              )}
              Delete Backup
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Upload Restore Confirmation Dialog */}
      <Dialog open={showUploadRestoreConfirm} onOpenChange={setShowUploadRestoreConfirm}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-amber-500">
              <AlertTriangle className="h-5 w-5" />
              Restore from Uploaded Backup
            </DialogTitle>
            <DialogDescription className="pt-2">
              <span className="font-bold text-red-500 block mb-2">Warning: This will replace all current data!</span>
              Are you sure you want to restore from <span className="font-mono font-bold text-foreground">{selectedUploadFile?.name}</span>?
              All current projects, deployments, and configurations will be replaced.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => {
              setShowUploadRestoreConfirm(false);
              setSelectedUploadFile(null);
            }}>
              Cancel
            </Button>
            <Button
              onClick={handleUploadRestore}
              disabled={uploadingBackup}
              className="bg-amber-600 hover:bg-amber-700"
            >
              {uploadingBackup ? (
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
              ) : (
                <RotateCcw className="h-4 w-4 mr-2" />
              )}
              Restore Backup
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Backup Progress Dialog */}
      <Dialog open={showBackupProgress} onOpenChange={(open) => !creatingBackup && setShowBackupProgress(open)}>
        <DialogContent className="sm:max-w-2xl max-h-[80vh]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {creatingBackup ? (
                <Loader2 className="h-5 w-5 animate-spin text-emerald-500" />
              ) : (
                <Check className="h-5 w-5 text-emerald-500" />
              )}
              {creatingBackup ? 'Creating Backup...' : 'Backup Complete'}
            </DialogTitle>
          </DialogHeader>
          <div className="bg-black/90 rounded-lg p-4 font-mono text-xs text-green-400 max-h-[400px] overflow-y-auto">
            {backupProgress.map((line, i) => (
              <div key={i} className="whitespace-pre-wrap">{line}</div>
            ))}
          </div>
          {!creatingBackup && (
            <DialogFooter>
              <Button onClick={() => setShowBackupProgress(false)}>Close</Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

      {/* Restore Progress Dialog */}
      <Dialog open={showRestoreProgress} onOpenChange={(open) => !(restoringBackup || uploadingBackup) && setShowRestoreProgress(open)}>
        <DialogContent className="sm:max-w-2xl max-h-[80vh]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {(restoringBackup || uploadingBackup) ? (
                <Loader2 className="h-5 w-5 animate-spin text-amber-500" />
              ) : (
                <Check className="h-5 w-5 text-emerald-500" />
              )}
              {(restoringBackup || uploadingBackup) ? 'Restoring Backup...' : 'Restore Complete'}
            </DialogTitle>
          </DialogHeader>
          <div className="bg-black/90 rounded-lg p-4 font-mono text-xs text-green-400 max-h-[400px] overflow-y-auto">
            {restoreProgress.map((line, i) => (
              <div key={i} className="whitespace-pre-wrap">{line}</div>
            ))}
          </div>
          {!(restoringBackup || uploadingBackup) && (
            <DialogFooter>
              <Button onClick={() => setShowRestoreProgress(false)}>Close</Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

    </div>
  );
}

export default function SettingsPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen flex items-center justify-center bg-black">
        <Loader2 className="h-8 w-8 animate-spin text-cyan-500" />
      </div>
    }>
      <SettingsContent />
    </Suspense>
  );
}
