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

export namespace main {
	
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
	
	export class SimParams {
	    handlingEffect: number;
	    draftFactor: number;
	    fatigueRate: number;
	    attackThreshold: number;
	
	    static createFrom(source: any = {}) {
	        return new SimParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.handlingEffect = source["handlingEffect"];
	        this.draftFactor = source["draftFactor"];
	        this.fatigueRate = source["fatigueRate"];
	        this.attackThreshold = source["attackThreshold"];
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

