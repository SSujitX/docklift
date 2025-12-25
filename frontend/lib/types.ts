export interface Project {
  id: string;
  name: string;
  description: string | null;
  source_type: "upload" | "github";
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
  status: "queued" | "in_progress" | "success" | "failed";
  logs: string;
  created_at: string;
  finished_at: string | null;
}

export interface Port {
  port: number;
  project_id: string | null;
  is_locked: boolean;
}

export interface ProjectFile {
  name: string;
  path: string;
  type: "file" | "folder";
  size?: number;
  editable?: boolean;
  children?: ProjectFile[];
}


