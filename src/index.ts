import { Container, getContainer } from "@cloudflare/containers";

export interface Env {
	API: DurableObjectNamespace<PDEContainer>;
	DATABASE_URL: string;
	REDIS_ADDR: string;
	REDIS_PASSWORD: string;
	REDIS_TLS: string;
	REDIS_DB: string;
	GROQ_API_KEY: string;
	GROQ_MODEL: string;
	GROQ_BASE_URL: string;
	OIDC_ISSUER: string;
	OIDC_AUDIENCE: string;
	AUTH_INTROSPECT_URL: string;
	INTROSPECT_SECRET: string;
	CORS_ORIGINS: string;
	RATE_LIMIT_PER_MINUTE: string;
	LOG_LEVEL: string;
	LOG_FORMAT: string;
}

function apiEnvVars(env: Env): Record<string, string> {
	return {
		PORT: "8080",
		DATABASE_URL: env.DATABASE_URL,
		DB_SEARCH_PATH: "pde",
		REDIS_ADDR: env.REDIS_ADDR,
		REDIS_PASSWORD: env.REDIS_PASSWORD || "",
		REDIS_TLS: env.REDIS_TLS || "true",
		REDIS_DB: env.REDIS_DB || "0",
		GROQ_API_KEY: env.GROQ_API_KEY,
		GROQ_MODEL: env.GROQ_MODEL || "qwen/qwen3.6-27b",
		GROQ_BASE_URL: env.GROQ_BASE_URL || "https://api.groq.com/openai/v1",
		OIDC_ISSUER: env.OIDC_ISSUER,
		OIDC_AUDIENCE: env.OIDC_AUDIENCE || "personal-document-extractor",
		AUTH_INTROSPECT_URL: env.AUTH_INTROSPECT_URL || "",
		INTROSPECT_SECRET: env.INTROSPECT_SECRET || "",
		CORS_ORIGINS: env.CORS_ORIGINS || "https://kalke.dev,https://www.kalke.dev",
		RATE_LIMIT_PER_MINUTE: env.RATE_LIMIT_PER_MINUTE || "60",
		LOG_LEVEL: env.LOG_LEVEL || "info",
		LOG_FORMAT: env.LOG_FORMAT || "json",
	};
}

export class PDEContainer extends Container<Env> {
	defaultPort = 8080;
	sleepAfter = "10m";

	override onStart(): void {
		this.envVars = apiEnvVars(this.env);
	}
}

export default {
	async fetch(request: Request, env: Env): Promise<Response> {
		const container = getContainer(env.API, "primary");
		await container.startAndWaitForPorts({
			startOptions: { envVars: apiEnvVars(env) },
			cancellationOptions: { portReadyTimeoutMS: 120_000 },
		});
		return container.fetch(request);
	},
};
