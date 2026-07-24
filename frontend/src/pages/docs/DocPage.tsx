import { Navigate, useParams } from "react-router-dom";

import { Introduction } from "./components/Introduction";
import { Installation } from "./components/Installation";
import { GithubIntegration } from "./components/GithubIntegration";
import { AutoDeploy } from "./components/AutoDeploy";
import { Deployment } from "./components/Deployment";
import { DockerfileSection } from "./components/DockerfileSection";
import { CustomDomains } from "./components/CustomDomains";
import { EnvironmentVariables } from "./components/EnvironmentVariables";
import { SystemOverview } from "./components/SystemOverview";
import { WebTerminal } from "./components/WebTerminal";
import { ApiReference } from "./components/ApiReference";
import { FileManagement } from "./components/FileManagement";
import { PortManagement } from "./components/PortManagement";
import { ProfileManagement } from "./components/ProfileManagement";
import { BackupRestore } from "./components/BackupRestore";
import { ResetPassword } from "./components/ResetPassword";
import { UsefulCommands } from "./components/UsefulCommands";
import { Troubleshooting } from "./components/Troubleshooting";

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
  backup: BackupRestore,
  "reset-password": ResetPassword,
  commands: UsefulCommands,
  troubleshooting: Troubleshooting,
};

export default function DocSectionPage() {
  const { slug = "" } = useParams<{ slug: string }>();
  const Component = componentMap[slug];

  if (!Component) {
    return <Navigate to="/docs/introduction" replace />;
  }

  return (
    <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
      <Component />
    </div>
  );
}
