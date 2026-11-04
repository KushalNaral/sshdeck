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
	    }
	}

}

