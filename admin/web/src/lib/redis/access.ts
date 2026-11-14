import type { Namespace } from "@/lib/api/types";

/**
 * The namespace label the platform Redis's NetworkPolicy admits by. The
 * platform chart names it in values; this is the value it ships with, and the
 * one the API writes (admin/api/internal/kube/redisaccess.go).
 */
export const REDIS_ACCESS_LABEL = "home-lab.example/redis-access";

/** Whether the NetworkPolicy lets this namespace's pods connect. */
export function hasRedisAccess(namespace: Namespace): boolean {
	return namespace.labels?.[REDIS_ACCESS_LABEL] === "true";
}
