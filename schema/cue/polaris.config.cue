package polaris

#Duration: =~"^[0-9]+(ns|us|µs|ms|s|m|h)([0-9]+(ns|us|µs|ms|s|m|h))*$"
#HashedSecret: "" | =~"^sha256:.+" | =~"^\\$\\{[A-Z0-9_]+\\}$"
#Modality: "chat" | "embed" | "image" | "video" | "voice" | "audio" | "music" | "notes" | "podcast" | "translation" | "interpreting" | "files" | "batch"

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
		files?: {
			enabled?: bool
			ingestion?: {
				max_upload_bytes?: int & >=0
				allowed_mime?: [...string]
			}
			storage?: {
				inline_max_bytes?: int & >=0
				blob_store?: "none" | "disk" | "s3"
				disk_path?: string
				s3?: _
			}
			downloads?: {
				token_ttl?: #Duration
			}
			ssrf?: {
				allowed_schemes?: [...("http" | "https")]
				deny_hosts?: [...string]
			}
			materialization?: {
				inline_fallback_max?: int & >=0
			}
			understanding?: {
				enabled?: bool
				mode?: "disabled" | "explicit" | "auto_fallback"
				profile?: "fast" | "balanced" | "quality"
				max_bytes?: int & >=0
				max_text_chars?: int & >=0
				cache_artifacts?: bool
				chunking?: {
					enabled?: bool
					max_chars?: int & >=0
					overlap_chars?: int & >=0
				}
				processors?: [...{
					enabled?: bool
					name?: string
					backend?: "http" | "remote_http" | "tika"
					endpoint?: string
					method?: string
					version?: string
					timeout?: #Duration
					priority?: int
					mime_types?: [...string]
					file_classes?: [...("text" | "image" | "pdf" | "office_document" | "office_spreadsheet" | "office_presentation" | "audio" | "video" | "archive" | "unknown")]
					artifacts?: [...("text" | "metadata" | "ocr_text" | "layout" | "table" | "form" | "chunk" | "image_caption" | "image_metadata" | "embedding" | "raw")]
					capabilities?: [...("detect" | "text_extract" | "metadata" | "ocr" | "layout" | "table_extract" | "form_extract" | "image_caption" | "chunk" | "embed")]
					profiles?: [...("fast" | "balanced" | "quality")]
					headers?: [string]: string
					ocr?: bool
				}]
			}
		}
		pricing?: {
			file?: string
			reload_interval_seconds?: int & >=0
			fail_on_missing?: bool
		}
		observability?: _
		reliability?: _
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
