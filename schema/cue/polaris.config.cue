package polaris

#Duration: =~"^[0-9]+(ns|us|µs|ms|s|m|h)([0-9]+(ns|us|µs|ms|s|m|h))*$"
#HashedSecret: "" | =~"^sha256:.+" | =~"^\\$\\{[A-Z0-9_]+\\}$"
#Modality: "chat" | "embed" | "image" | "video" | "voice" | "audio" | "music" | "notes" | "podcast" | "translation" | "interpreting"

#Config: {
	version: 2
	imports?: [...string]

	runtime?: {
		server?: {
			host?: string
			port?: int & >=1 & <=65535
			read_timeout?: #Duration
			write_timeout?: #Duration
			shutdown_timeout?: #Duration
			max_body_bytes?: int & >0
			cors?: {
				enabled?: bool
				allowed_origins?: [...string]
				allowed_headers?: [...string]
				allowed_methods?: [...string]
				exposed_headers?: [...string]
				allow_credentials?: bool
				max_age?: #Duration
			}
		}
		auth?: {
			mode?: "none" | "static" | "external" | "virtual_keys" | "multi-user"
			bootstrap_admin_key_hash?: #HashedSecret
			admin_key_hash?: #HashedSecret
			static_keys?: [..._]
			external?: {
				provider?: "signed_headers"
				shared_secret?: string
				max_clock_skew?: #Duration
				cache_ttl?: #Duration
			}
		}
		store?: {
			driver?: "sqlite" | "postgres"
			dsn?: string
			max_connections?: int & >0
			log_retention_days?: int & >0
			log_buffer_size?: int & >0
			log_flush_interval?: #Duration
		}
		cache?: _
		control_plane?: {
			enabled?: bool
		}
		tools?: _
		mcp?: {
			enabled?: bool
		}
		pricing?: {
			file?: string
			reload_interval_seconds?: int & >=0
			fail_on_missing?: bool
		}
		observability?: _
	}

	providers?: [string]: {
		enabled?: bool
		credentials?: {
			api_key?: string
			access_key_id?: string
			access_key_secret?: string
			session_token?: string
			app_id?: string
			speech_api_key?: string
			speech_access_token?: string
			secret_key?: string
			project_name?: string
			project_id?: string
			location?: string
		}
		transport?: {
			base_url?: string
			control_base_url?: string
			timeout?: #Duration
			retry?: {
				max_attempts?: int & >=0
				backoff?: string
				initial_delay?: #Duration
			}
		}
		models?: {
			use?: [...string]
			overrides?: [string]: {
				modality?: #Modality
				capabilities?: [...string]
				...
			}
		}
	}

	routing?: {
		fallbacks?: [..._]
		aliases?: [string]: string
		selectors?: [string]: _
	}
}
