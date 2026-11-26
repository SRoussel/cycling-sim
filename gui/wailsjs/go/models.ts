export namespace app {
	
	export class CourseInfo {
	    id: number;
	    name: string;
	    distance: number;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new CourseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.distance = source["distance"];
	        this.type = source["type"];
	    }
	}
	export class CurvePoint {
	    seconds: number;
	    wkg: number;
	    watts: number;
	    wkgP10: number;
	    wkgP50: number;
	    wkgP90: number;
	    wP10: number;
	    wP50: number;
	    wP90: number;
	    pctWkg: number;
	    pctW: number;
	
	    static createFrom(source: any = {}) {
	        return new CurvePoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seconds = source["seconds"];
	        this.wkg = source["wkg"];
	        this.watts = source["watts"];
	        this.wkgP10 = source["wkgP10"];
	        this.wkgP50 = source["wkgP50"];
	        this.wkgP90 = source["wkgP90"];
	        this.wP10 = source["wP10"];
	        this.wP50 = source["wP50"];
	        this.wP90 = source["wP90"];
	        this.pctWkg = source["pctWkg"];
	        this.pctW = source["pctW"];
	    }
	}
	export class GCStanding {
	    position: number;
	    riderName: string;
	    team: string;
	    totalTime: number;
	    gap: number;
	
	    static createFrom(source: any = {}) {
	        return new GCStanding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.position = source["position"];
	        this.riderName = source["riderName"];
	        this.team = source["team"];
	        this.totalTime = source["totalTime"];
	        this.gap = source["gap"];
	    }
	}
	export class RiderInfo {
	    id: number;
	    name: string;
	    team: string;
	    nationality: string;
	    weight: number;
	    ftp: number;
	    sprint: number;
	    climbing: number;
	    tt: number;
	    descending: number;
	    endurance: number;
	    handling: number;
	    recovery: number;
	    punch: number;
	    repeatability: number;
	
	    static createFrom(source: any = {}) {
	        return new RiderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.team = source["team"];
	        this.nationality = source["nationality"];
	        this.weight = source["weight"];
	        this.ftp = source["ftp"];
	        this.sprint = source["sprint"];
	        this.climbing = source["climbing"];
	        this.tt = source["tt"];
	        this.descending = source["descending"];
	        this.endurance = source["endurance"];
	        this.handling = source["handling"];
	        this.recovery = source["recovery"];
	        this.punch = source["punch"];
	        this.repeatability = source["repeatability"];
	    }
	}
	export class InteractiveSessionInfo {
	    sessionId: string;
	    raceId: number;
	    stageRaceId: number;
	    controlledTeam: string;
	    allRiders: RiderInfo[];
	    teamRiders: RiderInfo[];
	
	    static createFrom(source: any = {}) {
	        return new InteractiveSessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.raceId = source["raceId"];
	        this.stageRaceId = source["stageRaceId"];
	        this.controlledTeam = source["controlledTeam"];
	        this.allRiders = this.convertValues(source["allRiders"], RiderInfo);
	        this.teamRiders = this.convertValues(source["teamRiders"], RiderInfo);
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
	export class MountainsStanding {
	    position: number;
	    riderName: string;
	    team: string;
	    points: number;
	    gap: number;
	
	    static createFrom(source: any = {}) {
	        return new MountainsStanding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.position = source["position"];
	        this.riderName = source["riderName"];
	        this.team = source["team"];
	        this.points = source["points"];
	        this.gap = source["gap"];
	    }
	}
	export class NewsInfo {
	    date: string;
	    text: string;
	    mine: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NewsInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.text = source["text"];
	        this.mine = source["mine"];
	    }
	}
	export class ObjectiveInfo {
	    text: string;
	    kind: string;
	    target: number;
	    current: number;
	    met: boolean;
	    decided: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ObjectiveInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.kind = source["kind"];
	        this.target = source["target"];
	        this.current = source["current"];
	        this.met = source["met"];
	        this.decided = source["decided"];
	    }
	}
	export class OfferReply {
	    ok: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new OfferReply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	    }
	}
	export class SeasonEventInfo {
	    id: number;
	    kind: string;
	    refId: number;
	    name: string;
	    category: string;
	    start: string;
	    end: string;
	    days: number;
	    done: boolean;
	    podium: string[];
	    mine: string;
	    points: number;
	
	    static createFrom(source: any = {}) {
	        return new SeasonEventInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.refId = source["refId"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.start = source["start"];
	        this.end = source["end"];
	        this.days = source["days"];
	        this.done = source["done"];
	        this.podium = source["podium"];
	        this.mine = source["mine"];
	        this.points = source["points"];
	    }
	}
	export class PlanEventInfo {
	    event: SeasonEventInfo;
	    planned: boolean;
	    peak: boolean;
	    formPct: number;
	    squad: number;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new PlanEventInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.event = this.convertValues(source["event"], SeasonEventInfo);
	        this.planned = source["planned"];
	        this.peak = source["peak"];
	        this.formPct = source["formPct"];
	        this.squad = source["squad"];
	        this.size = source["size"];
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
	export class PointsStanding {
	    position: number;
	    riderName: string;
	    team: string;
	    points: number;
	    gap: number;
	
	    static createFrom(source: any = {}) {
	        return new PointsStanding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.position = source["position"];
	        this.riderName = source["riderName"];
	        this.team = source["team"];
	        this.points = source["points"];
	        this.gap = source["gap"];
	    }
	}
	export class ProfileRiderRef {
	    id: number;
	    name: string;
	    team: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileRiderRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.team = source["team"];
	    }
	}
	export class ProfileStat {
	    key: string;
	    label: string;
	    value: number;
	    pct: number;
	    about: string;
	    unit: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.value = source["value"];
	        this.pct = source["pct"];
	        this.about = source["about"];
	        this.unit = source["unit"];
	    }
	}
	export class RaceInfo {
	    id: number;
	    name: string;
	    courseId: number;
	    courseName: string;
	    raceType: string;
	    date: string;
	    distance: number;
	    temperature: number;
	    windSpeed: number;
	
	    static createFrom(source: any = {}) {
	        return new RaceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.courseId = source["courseId"];
	        this.courseName = source["courseName"];
	        this.raceType = source["raceType"];
	        this.date = source["date"];
	        this.distance = source["distance"];
	        this.temperature = source["temperature"];
	        this.windSpeed = source["windSpeed"];
	    }
	}
	export class RacePlan {
	    leaderId: number;
	    break: string;
	    hardOn: string[];
	    attackOn: string;
	
	    static createFrom(source: any = {}) {
	        return new RacePlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.leaderId = source["leaderId"];
	        this.break = source["break"];
	        this.hardOn = source["hardOn"];
	        this.attackOn = source["attackOn"];
	    }
	}
	export class SimulationResult {
	    position: number;
	    riderId: number;
	    riderName: string;
	    team: string;
	    time: number;
	    gap: number;
	    handling: number;
	
	    static createFrom(source: any = {}) {
	        return new SimulationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.position = source["position"];
	        this.riderId = source["riderId"];
	        this.riderName = source["riderName"];
	        this.team = source["team"];
	        this.time = source["time"];
	        this.gap = source["gap"];
	        this.handling = source["handling"];
	    }
	}
	export class RaceResults {
	    raceId: number;
	    raceName: string;
	    results: SimulationResult[];
	
	    static createFrom(source: any = {}) {
	        return new RaceResults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.raceId = source["raceId"];
	        this.raceName = source["raceName"];
	        this.results = this.convertValues(source["results"], SimulationResult);
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
	
	export class RiderPlanInfo {
	    riderId: number;
	    name: string;
	    specialty: string;
	    maxPeaks: number;
	    events: PlanEventInfo[];
	
	    static createFrom(source: any = {}) {
	        return new RiderPlanInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.riderId = source["riderId"];
	        this.name = source["name"];
	        this.specialty = source["specialty"];
	        this.maxPeaks = source["maxPeaks"];
	        this.events = this.convertValues(source["events"], PlanEventInfo);
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
	export class RiderProfile {
	    id: number;
	    name: string;
	    team: string;
	    age: number;
	    specialty: string;
	    fieldSize: number;
	    field: string;
	    curve: CurvePoint[];
	    physical: ProfileStat[];
	    ratings: ProfileStat[];
	
	    static createFrom(source: any = {}) {
	        return new RiderProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.team = source["team"];
	        this.age = source["age"];
	        this.specialty = source["specialty"];
	        this.fieldSize = source["fieldSize"];
	        this.field = source["field"];
	        this.curve = this.convertValues(source["curve"], CurvePoint);
	        this.physical = this.convertValues(source["physical"], ProfileStat);
	        this.ratings = this.convertValues(source["ratings"], ProfileStat);
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
	
	export class StandingRow {
	    pos: number;
	    riderId: number;
	    name: string;
	    team: string;
	    points: number;
	    wins: number;
	    mine: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StandingRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pos = source["pos"];
	        this.riderId = source["riderId"];
	        this.name = source["name"];
	        this.team = source["team"];
	        this.points = source["points"];
	        this.wins = source["wins"];
	        this.mine = source["mine"];
	    }
	}
	export class SeasonInfo {
	    id: number;
	    name: string;
	    team: string;
	    year: number;
	    startYear: number;
	    phase: string;
	    date: string;
	    budget: number;
	
	    static createFrom(source: any = {}) {
	        return new SeasonInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.team = source["team"];
	        this.year = source["year"];
	        this.startYear = source["startYear"];
	        this.phase = source["phase"];
	        this.date = source["date"];
	        this.budget = source["budget"];
	    }
	}
	export class SeasonHub {
	    season: SeasonInfo;
	    wages: number;
	    nextCommitted: number;
	    nextRiders: number;
	    roster: number;
	    confidence: number;
	    objectives: ObjectiveInfo[];
	    events: SeasonEventInfo[];
	    next?: SeasonEventInfo;
	    riders: StandingRow[];
	    teams: StandingRow[];
	    news: NewsInfo[];
	    yearOver: boolean;
	    transferOpen: boolean;
	    projectedBudget: number;
	
	    static createFrom(source: any = {}) {
	        return new SeasonHub(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.season = this.convertValues(source["season"], SeasonInfo);
	        this.wages = source["wages"];
	        this.nextCommitted = source["nextCommitted"];
	        this.nextRiders = source["nextRiders"];
	        this.roster = source["roster"];
	        this.confidence = source["confidence"];
	        this.objectives = this.convertValues(source["objectives"], ObjectiveInfo);
	        this.events = this.convertValues(source["events"], SeasonEventInfo);
	        this.next = this.convertValues(source["next"], SeasonEventInfo);
	        this.riders = this.convertValues(source["riders"], StandingRow);
	        this.teams = this.convertValues(source["teams"], StandingRow);
	        this.news = this.convertValues(source["news"], NewsInfo);
	        this.yearOver = source["yearOver"];
	        this.transferOpen = source["transferOpen"];
	        this.projectedBudget = source["projectedBudget"];
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
	
	export class SeasonResultRow {
	    kind: string;
	    stage: number;
	    position: number;
	    riderId: number;
	    name: string;
	    team: string;
	    gap: number;
	    points: number;
	    mine: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SeasonResultRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.stage = source["stage"];
	        this.position = source["position"];
	        this.riderId = source["riderId"];
	        this.name = source["name"];
	        this.team = source["team"];
	        this.gap = source["gap"];
	        this.points = source["points"];
	        this.mine = source["mine"];
	    }
	}
	export class SeasonRiderInfo {
	    id: number;
	    name: string;
	    team: string;
	    nationality: string;
	    weight: number;
	    ftp: number;
	    sprint: number;
	    climbing: number;
	    tt: number;
	    descending: number;
	    endurance: number;
	    handling: number;
	    recovery: number;
	    punch: number;
	    repeatability: number;
	    age: number;
	    specialty: string;
	    wkg: number;
	    sprintW: number;
	    value: number;
	    salary: number;
	    contractEnd: number;
	    nextTeam: string;
	    nextSalary: number;
	    morale: number;
	    load: number;
	    raceDays: number;
	    points: number;
	    wins: number;
	    programme: string[];
	    ask: number;
	
	    static createFrom(source: any = {}) {
	        return new SeasonRiderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.team = source["team"];
	        this.nationality = source["nationality"];
	        this.weight = source["weight"];
	        this.ftp = source["ftp"];
	        this.sprint = source["sprint"];
	        this.climbing = source["climbing"];
	        this.tt = source["tt"];
	        this.descending = source["descending"];
	        this.endurance = source["endurance"];
	        this.handling = source["handling"];
	        this.recovery = source["recovery"];
	        this.punch = source["punch"];
	        this.repeatability = source["repeatability"];
	        this.age = source["age"];
	        this.specialty = source["specialty"];
	        this.wkg = source["wkg"];
	        this.sprintW = source["sprintW"];
	        this.value = source["value"];
	        this.salary = source["salary"];
	        this.contractEnd = source["contractEnd"];
	        this.nextTeam = source["nextTeam"];
	        this.nextSalary = source["nextSalary"];
	        this.morale = source["morale"];
	        this.load = source["load"];
	        this.raceDays = source["raceDays"];
	        this.points = source["points"];
	        this.wins = source["wins"];
	        this.programme = source["programme"];
	        this.ask = source["ask"];
	    }
	}
	
	export class SquadRider {
	    id: number;
	    name: string;
	    team: string;
	    nationality: string;
	    weight: number;
	    ftp: number;
	    sprint: number;
	    climbing: number;
	    tt: number;
	    descending: number;
	    endurance: number;
	    handling: number;
	    recovery: number;
	    punch: number;
	    repeatability: number;
	    age: number;
	    specialty: string;
	    wkg: number;
	    sprintW: number;
	    value: number;
	    salary: number;
	    contractEnd: number;
	    nextTeam: string;
	    nextSalary: number;
	    morale: number;
	    load: number;
	    raceDays: number;
	    points: number;
	    wins: number;
	    programme: string[];
	    ask: number;
	    fit: number;
	    teamRank: number;
	    loadPct: number;
	    formPct: number;
	    planned: boolean;
	    available: boolean;
	    why: string;
	
	    static createFrom(source: any = {}) {
	        return new SquadRider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.team = source["team"];
	        this.nationality = source["nationality"];
	        this.weight = source["weight"];
	        this.ftp = source["ftp"];
	        this.sprint = source["sprint"];
	        this.climbing = source["climbing"];
	        this.tt = source["tt"];
	        this.descending = source["descending"];
	        this.endurance = source["endurance"];
	        this.handling = source["handling"];
	        this.recovery = source["recovery"];
	        this.punch = source["punch"];
	        this.repeatability = source["repeatability"];
	        this.age = source["age"];
	        this.specialty = source["specialty"];
	        this.wkg = source["wkg"];
	        this.sprintW = source["sprintW"];
	        this.value = source["value"];
	        this.salary = source["salary"];
	        this.contractEnd = source["contractEnd"];
	        this.nextTeam = source["nextTeam"];
	        this.nextSalary = source["nextSalary"];
	        this.morale = source["morale"];
	        this.load = source["load"];
	        this.raceDays = source["raceDays"];
	        this.points = source["points"];
	        this.wins = source["wins"];
	        this.programme = source["programme"];
	        this.ask = source["ask"];
	        this.fit = source["fit"];
	        this.teamRank = source["teamRank"];
	        this.loadPct = source["loadPct"];
	        this.formPct = source["formPct"];
	        this.planned = source["planned"];
	        this.available = source["available"];
	        this.why = source["why"];
	    }
	}
	export class SquadInfo {
	    event: SeasonEventInfo;
	    size: number;
	    riders: SquadRider[];
	    points: director.PlanPoint[];
	    race?: RaceInfo;
	
	    static createFrom(source: any = {}) {
	        return new SquadInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.event = this.convertValues(source["event"], SeasonEventInfo);
	        this.size = source["size"];
	        this.riders = this.convertValues(source["riders"], SquadRider);
	        this.points = this.convertValues(source["points"], director.PlanPoint);
	        this.race = this.convertValues(source["race"], RaceInfo);
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
	
	export class StageRaceInfo {
	    id: number;
	    name: string;
	    stageCount: number;
	    totalDistanceKm: number;
	
	    static createFrom(source: any = {}) {
	        return new StageRaceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.stageCount = source["stageCount"];
	        this.totalDistanceKm = source["totalDistanceKm"];
	    }
	}
	export class StageRaceStageInfo {
	    stageNumber: number;
	    raceId: number;
	    raceName: string;
	    courseName: string;
	    distanceKm: number;
	    raceType: string;
	    date: string;
	    restDayBefore: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StageRaceStageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stageNumber = source["stageNumber"];
	        this.raceId = source["raceId"];
	        this.raceName = source["raceName"];
	        this.courseName = source["courseName"];
	        this.distanceKm = source["distanceKm"];
	        this.raceType = source["raceType"];
	        this.date = source["date"];
	        this.restDayBefore = source["restDayBefore"];
	    }
	}
	
	export class ValidationResult {
	    testName: string;
	    passed: boolean;
	    correlation: number;
	    top10Accuracy: number;
	    timeGapMAE: number;
	    matchedRiders: number;
	    isRealRace: boolean;
	    details: string;
	
	    static createFrom(source: any = {}) {
	        return new ValidationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.testName = source["testName"];
	        this.passed = source["passed"];
	        this.correlation = source["correlation"];
	        this.top10Accuracy = source["top10Accuracy"];
	        this.timeGapMAE = source["timeGapMAE"];
	        this.matchedRiders = source["matchedRiders"];
	        this.isRealRace = source["isRealRace"];
	        this.details = source["details"];
	    }
	}
	export class YouthStanding {
	    position: number;
	    riderName: string;
	    team: string;
	    totalTime: number;
	    gap: number;
	
	    static createFrom(source: any = {}) {
	        return new YouthStanding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.position = source["position"];
	        this.riderName = source["riderName"];
	        this.team = source["team"];
	        this.totalTime = source["totalTime"];
	        this.gap = source["gap"];
	    }
	}

}

export namespace director {
	
	export class PlanPoint {
	    key: string;
	    label: string;
	    kind: string;
	    startKm: number;
	    lengthKm: number;
	    toGoKm: number;
	
	    static createFrom(source: any = {}) {
	        return new PlanPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.startKm = source["startKm"];
	        this.lengthKm = source["lengthKm"];
	        this.toGoKm = source["toGoKm"];
	    }
	}

}

export namespace simulation {
	
	export class ProfilePoint {
	    km: number;
	    elev: number;
	
	    static createFrom(source: any = {}) {
	        return new ProfilePoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.km = source["km"];
	        this.elev = source["elev"];
	    }
	}
	export class RouteSector {
	    name: string;
	    startKm: number;
	    lengthKm: number;
	    stars: number;
	    surface: string;
	
	    static createFrom(source: any = {}) {
	        return new RouteSector(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.startKm = source["startKm"];
	        this.lengthKm = source["lengthKm"];
	        this.stars = source["stars"];
	        this.surface = source["surface"];
	    }
	}
	export class RouteProfile {
	    totalKm: number;
	    points: ProfilePoint[];
	    sectors?: RouteSector[];
	
	    static createFrom(source: any = {}) {
	        return new RouteProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalKm = source["totalKm"];
	        this.points = this.convertValues(source["points"], ProfilePoint);
	        this.sectors = this.convertValues(source["sectors"], RouteSector);
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

