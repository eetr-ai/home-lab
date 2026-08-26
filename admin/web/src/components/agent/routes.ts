/**
 * Where the agent may take you.
 *
 * Sent with every opening message rather than written into the agent's prompt,
 * because the app that owns these routes is the only thing that knows them: a list
 * baked into the definition — which ships inside the agent's image — would go
 * stale the first time a page was added here.
 *
 * A `{placeholder}` is a template the agent fills in from something it looked up.
 * Some paths carry one in the query string instead — `?namespace=` and
 * `?database=` — and those matter as much as the path does: a page scoped to a
 * database opens on whichever one is named, and on the first one in the list when
 * none is. Saying "the extensions page" while the operator is looking at the
 * wrong database is a worse answer than saying nothing, and the parameter is the
 * only way this list can prevent it.
 *
 * Keep the descriptions short and factual; they are read by a model deciding
 * whether a page answers the question it was asked, not by a person browsing.
 *
 * This is a hint and not a boundary. What the drawer will actually navigate to is
 * decided by `parseNavigateEvent`, which takes any site-relative path — so adding
 * a page here does not grant anything, and leaving one out does not withhold it.
 */

export interface RouteHint {
	path: string;
	description: string;
}

export const ROUTE_CATALOGUE: readonly RouteHint[] = [
	{ path: "/overview", description: "The dashboard: cluster health, nodes and storage at a glance." },

	{ path: "/kubernetes", description: "The cluster section." },
	{ path: "/kubernetes/nodes", description: "Every node, with conditions and CPU and memory usage." },
	{
		path: "/kubernetes/workloads?namespace={namespace}",
		description: "Deployments, StatefulSets and DaemonSets in one namespace, with replica counts. Restarting and scaling happen here. Omit the parameter for the default namespace.",
	},
	{
		path: "/kubernetes/workloads/{kind}/{name}",
		description: "One workload in detail: its pods, its rollout state, and its logs. `kind` is deployment, statefulset or daemonset.",
	},
	{ path: "/kubernetes/pods?namespace={namespace}", description: "Pods in one namespace, with phase and restart counts." },
	{ path: "/kubernetes/events?namespace={namespace}", description: "Recent events in one namespace — where a pod that will not start says why." },
	{ path: "/kubernetes/storage", description: "PersistentVolumes, claims and storage classes." },

	{ path: "/postgres", description: "The PostgreSQL section." },
	{ path: "/postgres/databases", description: "The databases. Creating and dropping one happens here." },
	{ path: "/postgres/roles", description: "The roles, and what each may do. Creating and dropping one happens here." },
	{ path: "/postgres/extensions?database={database}", description: "The extensions installed in one database, and installing another." },
	{ path: "/postgres/query?database={database}", description: "The read-only SQL console, opened on one database. Take somebody here when the answer is a query they should run or adjust themselves." },

	{ path: "/mongo", description: "The MongoDB section." },
	{ path: "/mongo/databases", description: "The databases. Creating and dropping one happens here." },
	{ path: "/mongo/collections?database={database}", description: "The collections in one database, and creating or dropping one." },
	{ path: "/mongo/users?database={database}", description: "The users on one database, and their roles." },
	{ path: "/mongo/query?database={database}", description: "The read-only document console, opened on one database." },
] as const;
