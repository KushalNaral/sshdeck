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

export namespace history {
	
	export class Version {
	    id: string;
	    server: string;
	    path: string;
	    time: number;
	    author: string;
	    login: string;
	    kind: string;
	    hash: string;
	    size: number;
	    note?: string;
	    pinned?: boolean;
	    remote?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Version(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.server = source["server"];
	        this.path = source["path"];
	        this.time = source["time"];
	        this.author = source["author"];
	        this.login = source["login"];
	        this.kind = source["kind"];
	        this.hash = source["hash"];
	        this.size = source["size"];
	        this.note = source["note"];
	        this.pinned = source["pinned"];
	        this.remote = source["remote"];
	    }
	}

}

export namespace main {
	
	export class ActionResult {
	    output: string;
	    exit: number;
	    sudo?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.output = source["output"];
	        this.exit = source["exit"];
	        this.sudo = source["sudo"];
	    }
	}
	export class ConnectResult {
	    ok: boolean;
	    problem?: string;
	    recovery?: string;
	    details?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.problem = source["problem"];
	        this.recovery = source["recovery"];
	        this.details = source["details"];
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
	export class FileEntry {
	    name: string;
	    path: string;
	    dir: boolean;
	    link: boolean;
	    size: number;
	    mode: string;
	    mtime: number;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.dir = source["dir"];
	        this.link = source["link"];
	        this.size = source["size"];
	        this.mode = source["mode"];
	        this.mtime = source["mtime"];
	    }
	}
	export class HistorySettings {
	    off: boolean;
	    noServerCopy: boolean;
	    keep: number;
	    days: number;
	    deny: string[];
	
	    static createFrom(source: any = {}) {
	        return new HistorySettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.off = source["off"];
	        this.noServerCopy = source["noServerCopy"];
	        this.keep = source["keep"];
	        this.days = source["days"];
	        this.deny = source["deny"];
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
	
	export class Listing {
	    path: string;
	    entries: FileEntry[];
	
	    static createFrom(source: any = {}) {
	        return new Listing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.entries = this.convertValues(source["entries"], FileEntry);
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
	export class MonitorSettings {
	    off: boolean;
	    sampleSeconds: number;
	    scanMinutes: number;
	    diskWarn: number;
	    diskCrit: number;
	    memWarn: number;
	    cpuWarn: number;
	    noNotify: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MonitorSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.off = source["off"];
	        this.sampleSeconds = source["sampleSeconds"];
	        this.scanMinutes = source["scanMinutes"];
	        this.diskWarn = source["diskWarn"];
	        this.diskCrit = source["diskCrit"];
	        this.memWarn = source["memWarn"];
	        this.cpuWarn = source["cpuWarn"];
	        this.noNotify = source["noNotify"];
	    }
	}
	export class Proc {
	    pid: number;
	    user: string;
	    cpu: number;
	    mem: number;
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new Proc(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.user = source["user"];
	        this.cpu = source["cpu"];
	        this.mem = source["mem"];
	        this.command = source["command"];
	    }
	}
	export class PushResult {
	    server: string;
	    name: string;
	    started: number;
	    conflicts?: string[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PushResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = source["server"];
	        this.name = source["name"];
	        this.started = source["started"];
	        this.conflicts = source["conflicts"];
	        this.error = source["error"];
	    }
	}
	export class RunResult {
	    server: string;
	    name: string;
	    state: string;
	    exit: number;
	    output: string;
	    truncated?: boolean;
	    ms: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = source["server"];
	        this.name = source["name"];
	        this.state = source["state"];
	        this.exit = source["exit"];
	        this.output = source["output"];
	        this.truncated = source["truncated"];
	        this.ms = source["ms"];
	        this.error = source["error"];
	    }
	}
	export class SearchHit {
	    path: string;
	    line?: number;
	    text?: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.line = source["line"];
	        this.text = source["text"];
	    }
	}
	export class TextFile {
	    path: string;
	    content: string;
	    version: string;
	    mode: string;
	    noHistory?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TextFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	        this.version = source["version"];
	        this.mode = source["mode"];
	        this.noHistory = source["noHistory"];
	    }
	}
	export class Transfer {
	    id: string;
	    serverId: string;
	    name: string;
	    up: boolean;
	    done: number;
	    total: number;
	    state: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Transfer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.serverId = source["serverId"];
	        this.name = source["name"];
	        this.up = source["up"];
	        this.done = source["done"];
	        this.total = source["total"];
	        this.state = source["state"];
	        this.error = source["error"];
	    }
	}
	export class TunnelState {
	    id: string;
	    state: string;
	    error?: string;
	    conns: number;
	
	    static createFrom(source: any = {}) {
	        return new TunnelState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.state = source["state"];
	        this.error = source["error"];
	        this.conns = source["conns"];
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
	export class UploadResult {
	    started: number;
	    conflicts?: string[];
	    paths?: string[];
	    dest?: string;
	    cancelled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UploadResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.started = source["started"];
	        this.conflicts = source["conflicts"];
	        this.paths = source["paths"];
	        this.dest = source["dest"];
	        this.cancelled = source["cancelled"];
	    }
	}

}

export namespace metrics {
	
	export class Alert {
	    id: string;
	    server: string;
	    kind: string;
	    subject: string;
	    severity: string;
	    text: string;
	    since: number;
	    resolved?: number;
	
	    static createFrom(source: any = {}) {
	        return new Alert(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.server = source["server"];
	        this.kind = source["kind"];
	        this.subject = source["subject"];
	        this.severity = source["severity"];
	        this.text = source["text"];
	        this.since = source["since"];
	        this.resolved = source["resolved"];
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
	export class Sample {
	    t: number;
	    hasRates: boolean;
	    cpu: number;
	    steal: number;
	    load1: number;
	    load5: number;
	    load15: number;
	    cores: number;
	    memPct: number;
	    memUsedMb: number;
	    memTotalMb: number;
	    swapPct: number;
	    netRx: number;
	    netTx: number;
	    diskR: number;
	    diskW: number;
	    disks: Disk[];
	
	    static createFrom(source: any = {}) {
	        return new Sample(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.t = source["t"];
	        this.hasRates = source["hasRates"];
	        this.cpu = source["cpu"];
	        this.steal = source["steal"];
	        this.load1 = source["load1"];
	        this.load5 = source["load5"];
	        this.load15 = source["load15"];
	        this.cores = source["cores"];
	        this.memPct = source["memPct"];
	        this.memUsedMb = source["memUsedMb"];
	        this.memTotalMb = source["memTotalMb"];
	        this.swapPct = source["swapPct"];
	        this.netRx = source["netRx"];
	        this.netTx = source["netTx"];
	        this.diskR = source["diskR"];
	        this.diskW = source["diskW"];
	        this.disks = this.convertValues(source["disks"], Disk);
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
	export class Tunnel {
	    id: string;
	    name: string;
	    kind: string;
	    listen: string;
	    target: string;
	    auto?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Tunnel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.listen = source["listen"];
	        this.target = source["target"];
	        this.auto = source["auto"];
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
	    historyCopy?: string;
	    tunnels?: Tunnel[];
	    jumpId?: string;
	
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
	        this.historyCopy = source["historyCopy"];
	        this.tunnels = this.convertValues(source["tunnels"], Tunnel);
	        this.jumpId = source["jumpId"];
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
	export class Snippet {
	    id: string;
	    name: string;
	    command: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new Snippet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.command = source["command"];
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
	    trustedPin?: string;
	    manual?: boolean;
	    loginAt?: string;
	
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
	        this.manual = source["manual"];
	        this.loginAt = source["loginAt"];
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

