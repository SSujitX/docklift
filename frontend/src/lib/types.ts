// TypeScript interfaces for API data models (Project, Deployment, Service, etc.)
export interface Project {
  id: string;
  name: string;
  description: string | null;
  source_type: "upload" | "github" | "managed";
  project_type: "app" | "database";
  build_type: BuildType;
  base_directory: string;
  dockerfile_path: string | null;
  internal_port: number;
  /** When true, publish host ports from the pool. Default false — use domains. */
  publish_host_port?: boolean;
  /** Managed DB engine when project_type=database */
  db_engine?: string | null;
  github_url: string | null;
  github_branch: string;
  domain: string | null;
  port: number | null;
  status: "pending" | "building" | "running" | "stopped" | "error" | "degraded";
  container_name: string | null;
  created_at: string;
  updated_at: string;
}

export type BuildType = "auto" | "dockerfile" | "railpack";

export interface BuildDetection {
  requestedType: BuildType;
  resolvedType: Exclude<BuildType, "auto">;
  baseDirectory: string;
  dockerfilePath: string | null;
  detected: string;
  manifests: string[];
}

export interface StorageMount {
  id: string;
  project_id: string;
  service_name: string;
  name: string;
  display_name: string;
  mount_path: string;
  created_at: string;
}

export interface Deployment {
  id: string;
  project_id: string;
  status: "queued" | "in_progress" | "success" | "failed" | "pending" | "cancelled";
  trigger?: string;
  commit_message?: string;
  commit_sha?: string | null;
  /** Map of service name → docklift image tag; required for Restore previous */
  image_tags?: Record<string, string> | null;
  logs: string;
  created_at: string;
  finished_at: string | null;
}

export interface Port {
  port: number;
  project_id: string | null;
  is_locked: boolean;
  project?: {
    id: string;
    name: string;
    status: string;
  };
}

export interface ProjectFile {
  name: string;
  path: string;
  type: "file" | "folder";
  size?: number;
  editable?: boolean;
  children?: ProjectFile[];
}

export interface Service {
  id: string;
  project_id: string;
  name: string;
  dockerfile_path: string;
  container_name: string | null;
  port: number | null;
  internal_port: number;
  domain: string | null;
  status: "pending" | "building" | "running" | "stopped" | "error" | "degraded";
  created_at: string;
}

export interface SslEvent {
  at: string;
  level: "info" | "success" | "warn" | "error";
  message: string;
}

export interface DomainDnsCheck {
  domain: string;
  status: "ok" | "mismatch" | "missing" | "unknown";
  a: string[];
  aaaa: string[];
  serverIp: string | null;
  message: string;
}

export interface EnvVariable {
  id: string;
  project_id: string;
  /** Empty string = shared to every service; otherwise Docker service name. */
  service_name?: string;
  key: string;
  value: string;
  is_build_arg: boolean;
  is_runtime: boolean;
  is_secret?: boolean;
  created_at: string;
}
