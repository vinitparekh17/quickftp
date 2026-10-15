// QuickFTP shared TypeScript types

export interface ConnectionDTO {
  id: number;
  name: string;
  host: string;
  port: number;
  protocol: string; // "ftp" or "ftps_explicit"
  username: string;
  password: string; // only sent when saving, never returned
  remoteRoot: string;
  tlsVerify: boolean;
  createdAt: string;
}

export interface ProjectDTO {
  id: number;
  name: string;
  projectType: string; // "angular" or "dotnet"
  localPath: string;
  buildCommand: string;
  outputRelPath: string;
  artifactRulesJson: string;
}

export interface DeploymentDTO {
  id: number;
  projectId: number;
  connectionId: number;
  status: string; // running, completed, failed, rolled_back
  currentPhase: string;
  backupDir: string;
  errorMessage: string;
  startedAt: string;
  finishedAt: string;
  cancelled?: boolean;
}

export interface DeploymentFileDTO {
  id: number;
  deploymentId: number;
  relativePath: string;
  stage: string;
  status: string;
  sizeBytes: number;
  sha256Local: string;
  errorDetail: string;
  updatedAt: string;
}

export interface DeployOptions {
  iisOfflinePage: string;
  iisShutdownDelay: number; // milliseconds
}

export type Page = 'dashboard' | 'connections' | 'projects' | 'deploy' | 'history' | 'settings';

export interface ToastMessage {
  id: number;
  type: 'success' | 'error' | 'info';
  message: string;
}

export interface DeployLog {
  stream: string;
  line: string;
}
