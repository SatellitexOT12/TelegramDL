export namespace config {
	
	export class ListenerChat {
	    id: number;
	    topic_id?: number;
	    topic_name?: string;
	    name: string;
	    auto_download: boolean;
	    f_photos: boolean;
	    f_videos: boolean;
	    f_audios: boolean;
	    f_docs: boolean;
	    f_stickers: boolean;
	    folder?: string;
	    topic_folder?: string;
	    name_mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new ListenerChat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.topic_id = source["topic_id"];
	        this.topic_name = source["topic_name"];
	        this.name = source["name"];
	        this.auto_download = source["auto_download"];
	        this.f_photos = source["f_photos"];
	        this.f_videos = source["f_videos"];
	        this.f_audios = source["f_audios"];
	        this.f_docs = source["f_docs"];
	        this.f_stickers = source["f_stickers"];
	        this.folder = source["folder"];
	        this.topic_folder = source["topic_folder"];
	        this.name_mode = source["name_mode"];
	    }
	}
	export class SpeedLimit {
	    value: number;
	    unit: string;
	
	    static createFrom(source: any = {}) {
	        return new SpeedLimit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.unit = source["unit"];
	    }
	}
	export class Config {
	    max_concurrent_downloads: number;
	    parallel_chunks: boolean;
	    chunk_workers: number;
	    download_folder: string;
	    organize_by_chat: boolean;
	    minimize_to_tray: boolean;
	    shutdown_when_done: boolean;
	    sound_notification: boolean;
	    color_id?: number;
	    loader_color_id?: number;
	    speed_limit: SpeedLimit;
	    listener_enabled: boolean;
	    listener_chats: ListenerChat[];
	    listener_chat_ids: number[];
	    language: string;
	    notification_bot_enabled: boolean;
	    notification_bot_token: string;
	    notification_chat_id: number;
	    notification_topic_id?: number;
	    bot_notifications: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_concurrent_downloads = source["max_concurrent_downloads"];
	        this.parallel_chunks = source["parallel_chunks"];
	        this.chunk_workers = source["chunk_workers"];
	        this.download_folder = source["download_folder"];
	        this.organize_by_chat = source["organize_by_chat"];
	        this.minimize_to_tray = source["minimize_to_tray"];
	        this.shutdown_when_done = source["shutdown_when_done"];
	        this.sound_notification = source["sound_notification"];
	        this.color_id = source["color_id"];
	        this.loader_color_id = source["loader_color_id"];
	        this.speed_limit = this.convertValues(source["speed_limit"], SpeedLimit);
	        this.listener_enabled = source["listener_enabled"];
	        this.listener_chats = this.convertValues(source["listener_chats"], ListenerChat);
	        this.listener_chat_ids = source["listener_chat_ids"];
	        this.language = source["language"];
	        this.notification_bot_enabled = source["notification_bot_enabled"];
	        this.notification_bot_token = source["notification_bot_token"];
	        this.notification_chat_id = source["notification_chat_id"];
	        this.notification_topic_id = source["notification_topic_id"];
	        this.bot_notifications = source["bot_notifications"];
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

export namespace main {
	
	export class ServerInfo {
	    host: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new ServerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	    }
	}

}

