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
  projectName?: string;
  projectType?: string;
  connectionId: number;
  connectionName?: string;
  connectionHost?: string;
  remoteRoot?: string;
  buildCommand?: string;
  outputRelPath?: string;
  status: string; // running, completed, failed, rolled_back, cancelled, interrupted
  currentPhase: string;
  backupDir: string;
  errorMessage: string;
  startedAt: string;
  finishedAt: string;
  cancelled?: boolean;
}

export type DeploymentExecutionStatus =
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'interrupted'
  | 'rolling_back'
  | 'rolled_back'
  | 'rollback_failed';

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

export type Page =
  | 'dashboard'
  | 'connections'
  | 'projects'
  | 'deploy'
  | 'history'
  | 'settings'
  | 'files';

export interface RemoteFileDTO {
  path: string;
  size: number;
  sizeKnown: boolean;
  isDir: boolean;
  modified?: string;
}
export interface TransferDTO {
  id: string;
  direction: string;
  connectionId: number;
  connectionName: string;
  remotePath: string;
  localPath: string;
  status: string;
  bytes: number;
  total: number;
  startedAt: string;
  finishedAt?: string;
  error?: string;
  verification?: string;
}

export interface ToastMessage {
  id: number;
  type: 'success' | 'error' | 'info';
  message: string;
}

export interface DeployLog {
  stream: string;
  line: string;
}
