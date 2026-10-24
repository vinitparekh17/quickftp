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
	    projectName: string;
	    projectType: string;
	    connectionId: number;
	    connectionName: string;
	    connectionHost: string;
	    remoteRoot: string;
	    buildCommand: string;
	    outputRelPath: string;
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
	        this.projectName = source["projectName"];
	        this.projectType = source["projectType"];
	        this.connectionId = source["connectionId"];
	        this.connectionName = source["connectionName"];
	        this.connectionHost = source["connectionHost"];
	        this.remoteRoot = source["remoteRoot"];
	        this.buildCommand = source["buildCommand"];
	        this.outputRelPath = source["outputRelPath"];
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
	export class RemoteDeleteItem {
	    path: string;
	    isDir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RemoteDeleteItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	    }
	}
	export class SettingsDTO {
	    backupDir: string;
	    downloadDir: string;
	    downloadConflict: string;
	    uploadConflict: string;
	    maxConcurrent: number;
	
	    static createFrom(source: any = {}) {
	        return new SettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backupDir = source["backupDir"];
	        this.downloadDir = source["downloadDir"];
	        this.downloadConflict = source["downloadConflict"];
	        this.uploadConflict = source["uploadConflict"];
	        this.maxConcurrent = source["maxConcurrent"];
	    }
	}

}

export namespace transfer {
	
	export class ConnectionSnapshot {
	    host: string;
	    port: number;
	    protocol: string;
	    username: string;
	    tlsVerify: boolean;
	    credentialKey: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.protocol = source["protocol"];
	        this.username = source["username"];
	        this.tlsVerify = source["tlsVerify"];
	        this.credentialKey = source["credentialKey"];
	    }
	}
	export class Activity {
	    id: string;
	    direction: string;
	    connectionId: number;
	    connectionName: string;
	    remotePath: string;
	    localPath: string;
	    status: string;
	    bytes: number;
	    total: number;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    finishedAt?: any;
	    error?: string;
	    verification?: string;
	    conflictPolicy?: string;
	    profile: ConnectionSnapshot;
	
	    static createFrom(source: any = {}) {
	        return new Activity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.direction = source["direction"];
	        this.connectionId = source["connectionId"];
	        this.connectionName = source["connectionName"];
	        this.remotePath = source["remotePath"];
	        this.localPath = source["localPath"];
	        this.status = source["status"];
	        this.bytes = source["bytes"];
	        this.total = source["total"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
	        this.error = source["error"];
	        this.verification = source["verification"];
	        this.conflictPolicy = source["conflictPolicy"];
	        this.profile = this.convertValues(source["profile"], ConnectionSnapshot);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class RemoteFile {
	    path: string;
	    size: number;
	    sizeKnown: boolean;
	    isDir: boolean;
	    // Go type: time
	    modified?: any;
	
	    static createFrom(source: any = {}) {
	        return new RemoteFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.size = source["size"];
	        this.sizeKnown = source["sizeKnown"];
	        this.isDir = source["isDir"];
	        this.modified = this.convertValues(source["modified"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

