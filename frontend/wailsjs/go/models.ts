export namespace main {
	
	export class ConnectionDTO {
	    id: number;
	    name: string;
	    host: string;
	    port: number;
	    protocol: string;
	    username: string;
	    password: string;
	    remoteRoot: string;
	    tlsVerify: boolean;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.protocol = source["protocol"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.remoteRoot = source["remoteRoot"];
	        this.tlsVerify = source["tlsVerify"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class DeployOptions {
	    iisOfflinePage: string;
	    iisShutdownDelay: number;
	
	    static createFrom(source: any = {}) {
	        return new DeployOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.iisOfflinePage = source["iisOfflinePage"];
	        this.iisShutdownDelay = source["iisShutdownDelay"];
	    }
	}
	export class DeploymentDTO {
	    id: number;
	    projectId: number;
	    connectionId: number;
	    status: string;
	    currentPhase: string;
	    backupDir: string;
	    errorMessage: string;
	    startedAt: string;
	    finishedAt: string;
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.connectionId = source["connectionId"];
	        this.status = source["status"];
	        this.currentPhase = source["currentPhase"];
	        this.backupDir = source["backupDir"];
	        this.errorMessage = source["errorMessage"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.cancelled = source["cancelled"];
	    }
	}
	export class DeploymentFileDTO {
	    id: number;
	    deploymentId: number;
	    relativePath: string;
	    stage: string;
	    status: string;
	    sizeBytes: number;
	    sha256Local: string;
	    errorDetail: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentFileDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.deploymentId = source["deploymentId"];
	        this.relativePath = source["relativePath"];
	        this.stage = source["stage"];
	        this.status = source["status"];
	        this.sizeBytes = source["sizeBytes"];
	        this.sha256Local = source["sha256Local"];
	        this.errorDetail = source["errorDetail"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class ProjectDTO {
	    id: number;
	    name: string;
	    projectType: string;
	    localPath: string;
	    buildCommand: string;
	    outputRelPath: string;
	    artifactRulesJson: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.projectType = source["projectType"];
	        this.localPath = source["localPath"];
	        this.buildCommand = source["buildCommand"];
	        this.outputRelPath = source["outputRelPath"];
	        this.artifactRulesJson = source["artifactRulesJson"];
	    }
	}
	export class SettingsDTO {
	    backupDir: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backupDir = source["backupDir"];
	    }
	}

}

