# The panel's API

Load this before your first `admin_read` of a conversation. It is the map, so you
do not rediscover the routes one 404 at a time.

`admin_read` takes a **path only**, beginning `/api`, and a `query` object. It is
`GET`-only — that is enforced by the runtime, not by this document — so every
route below that is not a GET is listed to tell you it is out of reach, not to
suggest it.

A 4xx comes back as data rather than an error. Read it and correct the call.

## Kubernetes

| Path | What it answers |
| --- | --- |
| `/api/kubernetes/summary` | The whole cluster at a glance. **Start here** when the question is broad. |
| `/api/kubernetes/nodes` | Every node: conditions, capacity, and CPU and memory usage. |
| `/api/kubernetes/storage` | PersistentVolumes, claims, and the storage classes. |
| `/api/kubernetes/namespaces` | The namespaces. |
| `/api/kubernetes/namespaces/{ns}/workloads` | Deployments, StatefulSets and DaemonSets in one list, with their replica counts. |
| `/api/kubernetes/namespaces/{ns}/workloads/{kind}/{name}` | One workload in full. `kind` is `deployment`, `statefulset` or `daemonset`. |
| `/api/kubernetes/namespaces/{ns}/pods` | The pods, with phase, restarts and container status. |
| `/api/kubernetes/namespaces/{ns}/events` | The namespace's events — where a pod that will not start says why. |
| `/api/kubernetes/namespaces/{ns}/pods/{pod}/logs` | A pod's log. See below. |

**Always bound a log read.** Pass `{"tail": 200}`, and never `follow` — you buffer
the whole response, so a followed stream is a call that does not return. Add
`{"container": "..."}` when the pod has more than one; without it the call fails
and tells you so. `{"previous": true}` reads the last terminated instance, which
is the one that holds the reason for a crash-loop.

Node disk usage is not always available. metrics-server reports CPU and memory
only, and disk comes from the kubelet — a grant this installation may not have.
Where it is missing you will see the node's allocatable ephemeral storage
instead, which is a scheduling ceiling and not a usage reading. Do not report it
as "disk used".

## PostgreSQL and MongoDB

| Path | What it answers |
| --- | --- |
| `/api/postgres/databases` | The databases, with owner and size. |
| `/api/postgres/databases/{db}/extensions` | Which extensions are installed in one. |
| `/api/postgres/roles` | The roles, and what each may do. |
| `/api/mongo/databases` | The databases, with their sizes. |
| `/api/mongo/databases/{db}/collections` | The collections in one. |
| `/api/mongo/databases/{db}/users` | The users on one, and their roles. |

Either section answers 404 for every route when it is not configured on this
installation. That means "not set up here", not "no databases".

## What you cannot call, and what to say instead

These exist and are not yours. Every one of them is a screen in the panel with a
person's hand on it, which is the point:

| Not yours | Where it happens |
| --- | --- |
| Restart or scale a workload | The Kubernetes → Workloads page |
| Create or drop a PostgreSQL database, role or extension | The PostgreSQL pages |
| Create or drop a Mongo database, collection or user | The MongoDB pages |
| Run SQL, or a Mongo `find` | The query console — both are POST, so `allowMethods` refuses them even though they only read |

When somebody asks for one, name the page, offer to take them there with
`navigate_to`, and say what you would press. Do not narrate a change you cannot
make as though you had made it.

## Who is asking

`/api/whoami` describes the operator whose token you are calling with. Useful
exactly once: when a call comes back 401 or 403 and you need to say whether the
problem is the route or the caller.
