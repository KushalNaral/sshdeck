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
	export class LockInfo {
	    enabled: boolean;
	    locked: boolean;
	    idleMinutes: number;
	    endOnLock: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LockInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.locked = source["locked"];
	        this.idleMinutes = source["idleMinutes"];
	        this.endOnLock = source["endOnLock"];
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
	export class Report {
	    os: string;
	    kernel: string;
	    uptime: string;
	    cpu: number;
	    memTotalMb: number;
	    memUsedMb: number;
	    disks: Disk[];
	    php: string;
	    webserver: string;
	    db: string;
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
	        this.php = source["php"];
	        this.webserver = source["webserver"];
	        this.db = source["db"];
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
	    os?: string;
	    php?: string;
	    phpTarget?: string;
	    webserver?: string;
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
	        this.os = source["os"];
	        this.php = source["php"];
	        this.phpTarget = source["phpTarget"];
	        this.webserver = source["webserver"];
	        this.projects = source["projects"];
	        this.note = source["note"];
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
	    }
	}

}

