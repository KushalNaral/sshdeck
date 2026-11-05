export namespace audit {
	
	export class Check {
	    ok: boolean;
	    entries: number;
	    badLine: number;
	
	    static createFrom(source: any = {}) {
	        return new Check(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.entries = source["entries"];
	        this.badLine = source["badLine"];
	    }
	}
	export class Entry {
	    ts: string;
	    actor: string;
	    event: string;
	    subject?: string;
	    detail?: string;
	    prev: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ts = source["ts"];
	        this.actor = source["actor"];
	        this.event = source["event"];
	        this.subject = source["subject"];
	        this.detail = source["detail"];
	        this.prev = source["prev"];
	    }
	}

}

export namespace main {
	
	export class ConnectResult {
	    ok: boolean;
	    problem?: string;
	    recovery?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.problem = source["problem"];
	        this.recovery = source["recovery"];
	    }
	}
	export class Dep {
	    name: string;
	    purpose: string;
	    installed: boolean;
	    installable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Dep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.purpose = source["purpose"];
	        this.installed = source["installed"];
	        this.installable = source["installable"];
	    }
	}
	export class ImportRow {
	    index: number;
	    line: number;
	    type: string;
	    name: string;
	    action: string;
	    hasPassword: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.line = source["line"];
	        this.type = source["type"];
	        this.name = source["name"];
	        this.action = source["action"];
	        this.hasPassword = source["hasPassword"];
	        this.error = source["error"];
	    }
	}
	export class ImportPreview {
	    token: string;
	    file: string;
	    rows: ImportRow[];
	
	    static createFrom(source: any = {}) {
	        return new ImportPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.file = source["file"];
	        this.rows = this.convertValues(source["rows"], ImportRow);
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
	export class ImportResult {
	    added: number;
	    updated: number;
	    failures: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.updated = source["updated"];
	        this.failures = source["failures"];
	    }
	}
	
	export class LockInfo {
	    enabled: boolean;
	    locked: boolean;
	    idleMinutes: number;
	    endOnLock: boolean;
	    autoReconnect: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LockInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.locked = source["locked"];
	        this.idleMinutes = source["idleMinutes"];
	        this.endOnLock = source["endOnLock"];
	        this.autoReconnect = source["autoReconnect"];
	    }
	}
	export class UpdateStatus {
	    version: string;
	    autoCheck: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UpdateStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.autoCheck = source["autoCheck"];
	    }
	}

}

export namespace probe {
	
	export class Container {
	    name: string;
	    image: string;
	    status: string;
	    ports: string;
	
	    static createFrom(source: any = {}) {
	        return new Container(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.image = source["image"];
	        this.status = source["status"];
	        this.ports = source["ports"];
	    }
	}
	export class Disk {
	    mount: string;
	    usedPct: number;
	
	    static createFrom(source: any = {}) {
	        return new Disk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mount = source["mount"];
	        this.usedPct = source["usedPct"];
	    }
	}
	export class Found {
	    name: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new Found(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	    }
	}
	export class Report {
	    os: string;
	    kernel: string;
	    uptime: string;
	    cpu: number;
	    memTotalMb: number;
	    memUsedMb: number;
	    disks: Disk[];
	    software: Found[];
	    failedUnits: string[];
	    containers: Container[];
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.kernel = source["kernel"];
	        this.uptime = source["uptime"];
	        this.cpu = source["cpu"];
	        this.memTotalMb = source["memTotalMb"];
	        this.memUsedMb = source["memUsedMb"];
	        this.disks = this.convertValues(source["disks"], Disk);
	        this.software = this.convertValues(source["software"], Found);
	        this.failedUnits = source["failedUnits"];
	        this.containers = this.convertValues(source["containers"], Container);
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

export namespace store {
	
	export class Login {
	    id: string;
	    label: string;
	    kind: string;
	    user?: string;
	    address?: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new Login(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.user = source["user"];
	        this.address = source["address"];
	        this.note = source["note"];
	    }
	}
	export class Software {
	    name: string;
	    version?: string;
	    target?: string;
	
	    static createFrom(source: any = {}) {
	        return new Software(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.target = source["target"];
	    }
	}
	export class Server {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    auth: string;
	    keyPath?: string;
	    vpnId: string;
	    vpnRequired?: boolean;
	    production?: boolean;
	    autoConnect?: boolean;
	    os?: string;
	    logins?: Login[];
	    software?: Software[];
	    projects?: string[];
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.auth = source["auth"];
	        this.keyPath = source["keyPath"];
	        this.vpnId = source["vpnId"];
	        this.vpnRequired = source["vpnRequired"];
	        this.production = source["production"];
	        this.autoConnect = source["autoConnect"];
	        this.os = source["os"];
	        this.logins = this.convertValues(source["logins"], Login);
	        this.software = this.convertValues(source["software"], Software);
	        this.projects = source["projects"];
	        this.note = source["note"];
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
	
	export class Task {
	    id: string;
	    title: string;
	    note?: string;
	    status: string;
	    tags: string[];
	    serverIds: string[];
	    due?: string;
	    created: number;
	    updated: number;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.note = source["note"];
	        this.status = source["status"];
	        this.tags = source["tags"];
	        this.serverIds = source["serverIds"];
	        this.due = source["due"];
	        this.created = source["created"];
	        this.updated = source["updated"];
	    }
	}
	export class VPN {
	    id: string;
	    name: string;
	    kind: string;
	    gateway: string;
	    port: number;
	    user: string;
	    note?: string;
	    trustedCert?: string;
	    trustedPin?: string;
	
	    static createFrom(source: any = {}) {
	        return new VPN(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.gateway = source["gateway"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.note = source["note"];
	        this.trustedCert = source["trustedCert"];
	        this.trustedPin = source["trustedPin"];
	    }
	}

}

export namespace update {
	
	export class Info {
	    current: string;
	    latest: string;
	    available: boolean;
	    notes: string;
	    pageUrl: string;
	    canInstall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.available = source["available"];
	        this.notes = source["notes"];
	        this.pageUrl = source["pageUrl"];
	        this.canInstall = source["canInstall"];
	    }
	}

}

export namespace vpn {
	
	export class CertInfo {
	    subject: string;
	    issuer: string;
	    names: string[];
	    // Go type: time
	    notBefore: any;
	    // Go type: time
	    notAfter: any;
	    expired: boolean;
	    selfSigned: boolean;
	    trustedBySystem: boolean;
	    sha256: string;
	    pin: string;
	    fingerprint: string;
	
	    static createFrom(source: any = {}) {
	        return new CertInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.subject = source["subject"];
	        this.issuer = source["issuer"];
	        this.names = source["names"];
	        this.notBefore = this.convertValues(source["notBefore"], null);
	        this.notAfter = this.convertValues(source["notAfter"], null);
	        this.expired = source["expired"];
	        this.selfSigned = source["selfSigned"];
	        this.trustedBySystem = source["trustedBySystem"];
	        this.sha256 = source["sha256"];
	        this.pin = source["pin"];
	        this.fingerprint = source["fingerprint"];
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

