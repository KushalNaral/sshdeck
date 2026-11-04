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
	    vpnId: string;
	    vpnRequired?: boolean;
	    os?: string;
	    php?: string;
	    phpTarget?: string;
	    webserver?: string;
	    projects?: string[];
	    stage: number;
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
	        this.vpnId = source["vpnId"];
	        this.vpnRequired = source["vpnRequired"];
	        this.os = source["os"];
	        this.php = source["php"];
	        this.phpTarget = source["phpTarget"];
	        this.webserver = source["webserver"];
	        this.projects = source["projects"];
	        this.stage = source["stage"];
	        this.note = source["note"];
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

