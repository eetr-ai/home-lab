# The panel's API

Load this before your first `admin_api` call of a conversation. It is the map, so
you do not rediscover the routes one 404 at a time.

`admin_api` takes a `method` (`GET` unless you say otherwise), a **path only**
beginning `/api`, an optional `query` object and an optional JSON `body`. A path
that is a full URL, or that tries to leave `/api`, is refused before any request
is made.

A 4xx comes back as data rather than an error. Read it and correct the call.

You call with the asking operator's own credential, so what you may do is what
they may do. Reading is most of it; the writes are listed below with the rest, and
the rules for them are in your prompt rather than here.

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

## The two consoles

Both read, and both are `POST` — which is exactly why the method is yours to
choose. A verb says nothing about whether a call changes anything:

| Path | |
| --- | --- |
| `POST /api/postgres/databases/{db}/query` | Run a read-only statement. Body `{"sql": "..."}`. It runs as a separate, deliberately non-superuser credential, and answers 503 where that is not configured. |
| `POST /api/mongo/databases/{db}/find` | Read documents from a collection. |

## What changes something

These work, and you may use them. The rules for how are in your prompt: say what
you are about to do first, make one change at a time, and never guess an
identifier for a destructive call.

| Path | |
| --- | --- |
| `POST /api/kubernetes/namespaces/{ns}/workloads/{kind}/{name}/restart` | Roll the pods. |
| `PUT /api/kubernetes/namespaces/{ns}/workloads/{kind}/{name}/scale` | Set the replica count. |
| `POST` / `PUT` / `DELETE` on `/api/postgres/databases`, `/api/postgres/roles` | Create, alter the owner, drop. |
| `POST /api/postgres/databases/{db}/extensions` | Install an extension. |
| `POST` / `PUT` / `DELETE` on `/api/mongo/databases`, its collections and users | The same, for MongoDB. |

Every one of these also has a screen in the panel. When there is no hurry, that is
the better answer: name the page, offer to take them there with `navigate_to`, and
say what you would press. A person watching a form is better placed to notice they
meant something else. Dropping a database is the clearest case — offer the page.

Never narrate a change as done before you have made it and read what came back.

## Who is asking

`/api/whoami` describes the operator whose token you are calling with. Useful
exactly once: when a call comes back 401 or 403 and you need to say whether the
problem is the route or the caller.
