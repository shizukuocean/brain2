export namespace agentconnect {

	export class Detection {
	    id: string;
	    name: string;
	    surface: string;
	    detected: boolean;
	    quick_connect: boolean;
	    prompt_connect: boolean;
	    executable: string;
	    version: string;
	    status: string;
	    message: string;
	    mcp_status: string;
	    mcp_message: string;
	    mcp_version: string;
	    mcp_available_version: string;
	    rollback_available: boolean;

	    static createFrom(source: any = {}) {
	        return new Detection(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.surface = source["surface"];
	        this.detected = source["detected"];
	        this.quick_connect = source["quick_connect"];
	        this.prompt_connect = source["prompt_connect"];
	        this.executable = source["executable"];
	        this.version = source["version"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.mcp_status = source["mcp_status"];
	        this.mcp_message = source["mcp_message"];
	        this.mcp_version = source["mcp_version"];
	        this.mcp_available_version = source["mcp_available_version"];
	        this.rollback_available = source["rollback_available"];
	    }
	}
	export class Result {
	    agent_id: string;
	    status: string;
	    connection_id: string;
	    message: string;
	    prompt: string;
	    receipt_path: string;
	    changed_paths: string[];
	    restart_required: boolean;
	    skill_updated: boolean;
	    host_updated: boolean;
	    config_updated: boolean;
	    skill_conflict: boolean;
	    verification_prompt: string;

	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agent_id = source["agent_id"];
	        this.status = source["status"];
	        this.connection_id = source["connection_id"];
	        this.message = source["message"];
	        this.prompt = source["prompt"];
	        this.receipt_path = source["receipt_path"];
	        this.changed_paths = source["changed_paths"];
	        this.restart_required = source["restart_required"];
	        this.skill_updated = source["skill_updated"];
	        this.host_updated = source["host_updated"];
	        this.config_updated = source["config_updated"];
	        this.skill_conflict = source["skill_conflict"];
	        this.verification_prompt = source["verification_prompt"];
	    }
	}

}

export namespace brain2 {

	export class CheckResult {
	    name: string;
	    status: string;
	    message: string;
	    path: string;
	    category?: string;

	    static createFrom(source: any = {}) {
	        return new CheckResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.path = source["path"];
	        this.category = source["category"];
	    }
	}
	export class HealthCategorySummary {
	    category: string;
	    status: string;
	    ok: number;
	    warning: number;
	    error: number;
	    info: number;

	    static createFrom(source: any = {}) {
	        return new HealthCategorySummary(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.status = source["status"];
	        this.ok = source["ok"];
	        this.warning = source["warning"];
	        this.error = source["error"];
	        this.info = source["info"];
	    }
	}
	export class HealthReport {
	    schema_version: string;
	    generated_at: string;
	    overall_status: string;
	    score: number;
	    brain2_root: string;
	    workspace_id: string;
	    mcp_host_version?: string;
	    mcp_instance_id?: string;
	    mcp_started_at?: string;
	    findings_total: number;
	    returned_checks: number;
	    checks_truncated: boolean;
	    categories: HealthCategorySummary[];
	    checks: CheckResult[];

	    static createFrom(source: any = {}) {
	        return new HealthReport(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema_version = source["schema_version"];
	        this.generated_at = source["generated_at"];
	        this.overall_status = source["overall_status"];
	        this.score = source["score"];
	        this.brain2_root = source["brain2_root"];
	        this.workspace_id = source["workspace_id"];
	        this.mcp_host_version = source["mcp_host_version"];
	        this.mcp_instance_id = source["mcp_instance_id"];
	        this.mcp_started_at = source["mcp_started_at"];
	        this.findings_total = source["findings_total"];
	        this.returned_checks = source["returned_checks"];
	        this.checks_truncated = source["checks_truncated"];
	        this.categories = this.convertValues(source["categories"], HealthCategorySummary);
	        this.checks = this.convertValues(source["checks"], CheckResult);
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
	export class ProjectCandidate {
	    id: string;
	    scan_task_id: string;
	    agent_id: string;
	    name: string;
	    root_path: string;
	    suggested_goal: string;
	    confidence: number;
	    evidence: string[];
	    detected_kind: string;
	    suggested_consumers: string[];
	    parent_project_id?: string;
	    status: string;
	    submitted_at: string;

	    static createFrom(source: any = {}) {
	        return new ProjectCandidate(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.scan_task_id = source["scan_task_id"];
	        this.agent_id = source["agent_id"];
	        this.name = source["name"];
	        this.root_path = source["root_path"];
	        this.suggested_goal = source["suggested_goal"];
	        this.confidence = source["confidence"];
	        this.evidence = source["evidence"];
	        this.detected_kind = source["detected_kind"];
	        this.suggested_consumers = source["suggested_consumers"];
	        this.parent_project_id = source["parent_project_id"];
	        this.status = source["status"];
	        this.submitted_at = source["submitted_at"];
	    }
	}

}

export namespace main {

	export class AgentSummary {
	    id: string;
	    name: string;
	    surface: string;
	    status: string;
	    mode: string;
	    connected_at: string;
	    last_error: string;

	    static createFrom(source: any = {}) {
	        return new AgentSummary(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.surface = source["surface"];
	        this.status = source["status"];
	        this.mode = source["mode"];
	        this.connected_at = source["connected_at"];
	        this.last_error = source["last_error"];
	    }
	}
	export class DeviceSummary {
	    id: string;
	    name: string;
	    brain2_path: string;
	    machine_name: string;
	    local: boolean;
	    status: string;

	    static createFrom(source: any = {}) {
	        return new DeviceSummary(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.brain2_path = source["brain2_path"];
	        this.machine_name = source["machine_name"];
	        this.local = source["local"];
	        this.status = source["status"];
	    }
	}
	export class HelperLaunchResult {
	    status: string;
	    message: string;
	    task_id: string;
	    project_id: string;
	    product_version: string;
	    accepted_scopes: string[];
	    expires_at: string;

	    static createFrom(source: any = {}) {
	        return new HelperLaunchResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.message = source["message"];
	        this.task_id = source["task_id"];
	        this.project_id = source["project_id"];
	        this.product_version = source["product_version"];
	        this.accepted_scopes = source["accepted_scopes"];
	        this.expires_at = source["expires_at"];
	    }
	}
	export class ProjectSummary {
	    id: string;
	    name: string;
	    mode: string;
	    status: string;
	    source_path: string;
	    manifest_path: string;
	    asset_status: string;
	    path_status: string;
	    truth_status: string;
	    machine_record: boolean;
	    protector_eligible: boolean;
	    helper_eligible: boolean;

	    static createFrom(source: any = {}) {
	        return new ProjectSummary(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.mode = source["mode"];
	        this.status = source["status"];
	        this.source_path = source["source_path"];
	        this.manifest_path = source["manifest_path"];
	        this.asset_status = source["asset_status"];
	        this.path_status = source["path_status"];
	        this.truth_status = source["truth_status"];
	        this.machine_record = source["machine_record"];
	        this.protector_eligible = source["protector_eligible"];
	        this.helper_eligible = source["helper_eligible"];
	    }
	}
	export class LauncherState {
	    brain2_root: string;
	    workspace: boolean;
	    project_count: number;
	    agent_count: number;
	    runtime_ready: boolean;
	    health_status: string;
	    health_score: number;
	    projects: ProjectSummary[];
	    agents: AgentSummary[];
	    launcher_build: string;
	    launcher_profile: string;
	    obsidian_ready: boolean;
	    obsidian_path: string;
	    acceptance_mode: string;
	    project_candidates: brain2.ProjectCandidate[];
	    protector_available: boolean;
	    helper_available: boolean;
	    local_device_id: string;
	    selected_device_id: string;
	    selected_device_local: boolean;
	    device_setup_required: boolean;
	    local_device_source: string;
	    devices: DeviceSummary[];

	    static createFrom(source: any = {}) {
	        return new LauncherState(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.brain2_root = source["brain2_root"];
	        this.workspace = source["workspace"];
	        this.project_count = source["project_count"];
	        this.agent_count = source["agent_count"];
	        this.runtime_ready = source["runtime_ready"];
	        this.health_status = source["health_status"];
	        this.health_score = source["health_score"];
	        this.projects = this.convertValues(source["projects"], ProjectSummary);
	        this.agents = this.convertValues(source["agents"], AgentSummary);
	        this.launcher_build = source["launcher_build"];
	        this.launcher_profile = source["launcher_profile"];
	        this.obsidian_ready = source["obsidian_ready"];
	        this.obsidian_path = source["obsidian_path"];
	        this.acceptance_mode = source["acceptance_mode"];
	        this.project_candidates = this.convertValues(source["project_candidates"], brain2.ProjectCandidate);
	        this.protector_available = source["protector_available"];
	        this.helper_available = source["helper_available"];
	        this.local_device_id = source["local_device_id"];
	        this.selected_device_id = source["selected_device_id"];
	        this.selected_device_local = source["selected_device_local"];
	        this.device_setup_required = source["device_setup_required"];
	        this.local_device_source = source["local_device_source"];
	        this.devices = this.convertValues(source["devices"], DeviceSummary);
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
	export class ProjectArchiveBatchItem {
	    task_id: string;
	    candidate_id: string;
	    project_name: string;

	    static createFrom(source: any = {}) {
	        return new ProjectArchiveBatchItem(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.candidate_id = source["candidate_id"];
	        this.project_name = source["project_name"];
	    }
	}
	export class ProjectArchiveBatchResult {
	    message: string;
	    prompt: string;
	    items: ProjectArchiveBatchItem[];

	    static createFrom(source: any = {}) {
	        return new ProjectArchiveBatchResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.prompt = source["prompt"];
	        this.items = this.convertValues(source["items"], ProjectArchiveBatchItem);
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
	export class ProjectArchiveResult {
	    task_id: string;
	    message: string;
	    prompt: string;
	    candidate_id: string;
	    project_name: string;

	    static createFrom(source: any = {}) {
	        return new ProjectArchiveResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.message = source["message"];
	        this.prompt = source["prompt"];
	        this.candidate_id = source["candidate_id"];
	        this.project_name = source["project_name"];
	    }
	}
	export class ProjectScanResult {
	    task_id: string;
	    message: string;
	    prompt: string;
	    brain2_root: string;
	    workspace_id: string;
	    scan_root: string;

	    static createFrom(source: any = {}) {
	        return new ProjectScanResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.message = source["message"];
	        this.prompt = source["prompt"];
	        this.brain2_root = source["brain2_root"];
	        this.workspace_id = source["workspace_id"];
	        this.scan_root = source["scan_root"];
	    }
	}

	export class ProtectorCatalogLaunchResult {
	    status: string;
	    message: string;
	    catalog_id: string;
	    project_count: number;
	    product_version: string;
	    expires_at: string;

	    static createFrom(source: any = {}) {
	        return new ProtectorCatalogLaunchResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.message = source["message"];
	        this.catalog_id = source["catalog_id"];
	        this.project_count = source["project_count"];
	        this.product_version = source["product_version"];
	        this.expires_at = source["expires_at"];
	    }
	}

}
