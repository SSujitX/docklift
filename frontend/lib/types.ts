// TypeScript interfaces for API data models (Project, Deployment, Service, etc.)
export interface Project {
  id: string;
  name: string;
  description: string | null;
  source_type: "upload" | "github";
  project_type: "app" | "database";
  github_url: string | null;
  github_branch: string;
  domain: string | null;
  port: number | null;
  status: "pending" | "building" | "running" | "stopped" | "error";
  container_name: string | null;
  created_at: string;
  updated_at: string;
}

export interface Deployment {
  id: string;
  project_id: string;
  status: "queued" | "in_progress" | "success" | "failed" | "pending";
  trigger?: string;
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
  status: "pending" | "building" | "running" | "stopped" | "error";
  created_at: string;
}

export interface EnvVariable {
  id: string;
  project_id: string;
  key: string;
  value: string;
  is_build_arg: boolean;
  is_runtime: boolean;
  created_at: string;
}
