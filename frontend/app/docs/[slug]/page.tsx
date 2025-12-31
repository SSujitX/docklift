"use client";

import { use } from "react";
import { notFound } from "next/navigation";

// Import modular sections from local components directory
import { Introduction } from "../components/Introduction";
import { Installation } from "../components/Installation";
import { GithubIntegration } from "../components/GithubIntegration";
import { AutoDeploy } from "../components/AutoDeploy";
import { Deployment } from "../components/Deployment";
import { DockerfileSection } from "../components/DockerfileSection";
import { CustomDomains } from "../components/CustomDomains";
import { EnvironmentVariables } from "../components/EnvironmentVariables";
import { SystemOverview } from "../components/SystemOverview";
import { WebTerminal } from "../components/WebTerminal";
import { ApiReference } from "../components/ApiReference";
import { FileManagement } from "../components/FileManagement";
import { PortManagement } from "../components/PortManagement";
import { ProfileManagement } from "../components/ProfileManagement";
import { ResetPassword } from "../components/ResetPassword";
import { UsefulCommands } from "../components/UsefulCommands";
import { Troubleshooting } from "../components/Troubleshooting";

const componentMap: Record<string, React.ComponentType> = {
  introduction: Introduction,
  installation: Installation,
  github: GithubIntegration,
  autodeploy: AutoDeploy,
  deployment: Deployment,
  dockerfile: DockerfileSection,
  domains: CustomDomains,
  environment: EnvironmentVariables,
  system: SystemOverview,
  terminal: WebTerminal,
  api: ApiReference,
  files: FileManagement,
  ports: PortManagement,
  profile: ProfileManagement,
  "reset-password": ResetPassword,
  commands: UsefulCommands,
  troubleshooting: Troubleshooting,
};

export default function DocSectionPage({ params }: { params: Promise<{ slug: string }> | { slug: string } }) {
  // Handle both Promise and synchronous params (Next.js 15+ vs 14-)
  const resolvedParams = params instanceof Promise ? use(params) : params;
  const { slug } = resolvedParams;
  
  const Component = componentMap[slug];

  if (!Component) {
    notFound();
  }

  return (
    <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
      <Component />
    </div>
  );
}
