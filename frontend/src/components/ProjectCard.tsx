// ProjectCard component - one project as a list row: name, status, manifest line and actions.

import { Project } from "@/lib/types";
import { ProjectQuickActions } from "@/components/project/ProjectQuickActions";
import { StatusBadge } from "@/components/StatusBadge";
import { useNavigate } from "react-router-dom";

interface ProjectCardProps {
  project: Project;
  onRefresh: () => void;
}

export function ProjectCard({ project, onRefresh }: ProjectCardProps) {
  const navigate = useNavigate();

  const handleCardClick = () => {
    navigate(`/projects/${project.id}`);
  };

  const facts: string[] = [];
  if (project.github_branch) facts.push(project.github_branch);
  if (project.port) facts.push(`:${project.port}`);

  return (
    <div
      onClick={handleCardClick}
      role="link"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) return;
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          handleCardClick();
        }
      }}
      className="flex cursor-pointer flex-col gap-3 rounded-lg border border-border bg-card px-4 py-3 transition-colors duration-150 hover:bg-secondary/50 focus-visible:-outline-offset-2 md:flex-row md:items-center md:justify-between"
    >
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-3">
          <h3 className="truncate text-sm font-medium leading-5 text-foreground">
            {project.name}
          </h3>
          <StatusBadge status={project.status} />
        </div>
        <p className="mt-0.5 truncate font-mono text-xs leading-4 text-muted-foreground">
          {facts.map((fact, i) => (
            <span key={`${i}-${fact}`}>
              {i > 0 && <span className="text-subtle"> · </span>}
              {fact}
            </span>
          ))}
        </p>
        <p className="mt-1 text-xs leading-4 text-muted-foreground">
          <span title={new Date(project.created_at).toLocaleString()}>
            Created {new Date(project.created_at).toLocaleDateString()}
          </span>
          <span className="text-subtle"> · </span>
          <span title={new Date(project.updated_at).toLocaleString()}>
            Updated {new Date(project.updated_at).toLocaleDateString()}
          </span>
        </p>
      </div>

      <ProjectQuickActions project={project} onRefresh={onRefresh} />
    </div>
  );
}
