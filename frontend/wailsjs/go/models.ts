export namespace email {
	
	export class MoeMailConfig {
	    name: string;
	    url: string;
	    apiKey: string;
	
	    static createFrom(source: any = {}) {
	        return new MoeMailConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	        this.apiKey = source["apiKey"];
	    }
	}

}

export namespace proxy {
	
	export class Info {
	    ok: boolean;
	    scheme: string;
	    ip: string;
	    country: string;
	    region: string;
	    city: string;
	    isp: string;
	    error?: string;
	    templated?: boolean;
	    pool?: boolean;
	    attempts?: number;
	    successAttempt?: number;
	    target?: string;
	    durationMs?: number;
	    errors?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.scheme = source["scheme"];
	        this.ip = source["ip"];
	        this.country = source["country"];
	        this.region = source["region"];
	        this.city = source["city"];
	        this.isp = source["isp"];
	        this.error = source["error"];
	        this.templated = source["templated"];
	        this.pool = source["pool"];
	        this.attempts = source["attempts"];
	        this.successAttempt = source["successAttempt"];
	        this.target = source["target"];
	        this.durationMs = source["durationMs"];
	        this.errors = source["errors"];
	    }
	}

}

export namespace task {
	
	export class StartTaskRequest {
	    count: number;
	    concurrency: number;
	    delay: number;
	    outputPath: string;
	    emailProvider: string;
	    moemailDomains: string[];
	    moemailConfigs: Record<string, Array<email.MoeMailConfig>>;
	    moemailRandomMode: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StartTaskRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.count = source["count"];
	        this.concurrency = source["concurrency"];
	        this.delay = source["delay"];
	        this.outputPath = source["outputPath"];
	        this.emailProvider = source["emailProvider"];
	        this.moemailDomains = source["moemailDomains"];
	        this.moemailConfigs = this.convertValues(source["moemailConfigs"], Array<email.MoeMailConfig>, true);
	        this.moemailRandomMode = source["moemailRandomMode"];
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

