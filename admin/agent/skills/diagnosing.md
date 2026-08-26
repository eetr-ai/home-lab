# Working out what is wrong

Load this when something is broken, slow, or not where it should be. It is a
reading order, not a script — but taking the steps out of order is how an answer
ends up confidently about the wrong layer.

Drop the cooking register for the whole of this. Somebody with a broken cluster
is not in the mood.

## A pod that will not start

1. `/api/kubernetes/namespaces/{ns}/pods` — read the phase and the container
   status. The reason is usually already there: `ImagePullBackOff`,
   `CreateContainerConfigError`, `CrashLoopBackOff`, `Pending`.
2. `/api/kubernetes/namespaces/{ns}/events` — the events say what the phase
   means. A `Pending` pod's event names what could not be satisfied; a
   `FailedMount` names the volume.
3. The logs, with `{"tail": 200}`. For a crash-loop add `{"previous": true}` —
   the running container has just started and holds nothing; the one that died
   holds the reason.

What each of those usually means here:

- **ImagePullBackOff** — the tag does not exist, or the node has no credential
  for that registry. Registry credentials on this cluster are held by the
  kubelet, cluster-wide, rather than as a per-namespace pull secret, so "add an
  imagePullSecret" is usually the wrong advice.
- **CreateContainerConfigError** — almost always a Secret or a key inside one
  that is not there. The chart references Secrets by name and they are created
  outside Helm, so a fresh install missing one fails exactly this way.
- **Pending with no node** — read `/api/kubernetes/nodes` for pressure and
  capacity before blaming the workload.
- **FailedMount** — read `/api/kubernetes/storage`. A claim that is `Pending` has
  no volume; a claim that is `Bound` and still will not mount is the NFS server.

## A rollout that is not finishing

Read the workload, not the pods. `/api/kubernetes/namespaces/{ns}/workloads/{kind}/{name}`
carries the desired, updated, ready and available counts, and the difference
between them is the whole diagnosis: updated but not ready is the new pod
failing, ready but not available is a readiness probe with a minimum age, and
nothing updated at all is a rollout that never started.

A Deployment with `strategy: Recreate` — the agent's own is one — has a gap with
no pods at all during a rollout. That is not a fault.

## "The site is down"

Work outwards, and stop at the first layer that is wrong:

1. Is the pod running and ready?
2. Is there a Service, and does its selector match the pod's labels?
3. Is there an HTTPRoute, and does it name the `home-lab` Gateway in
   `platform-system`?
4. Does the namespace carry `home-lab.example/gateway-access=true`? Without it
   the Gateway will not accept the route, and nothing in the route says so.
5. Cloudflare Access, and DNS. Neither is visible from here — say that rather
   than guessing.

## Reporting it

Say which call each fact came from. Say what you could not check. If the events
are empty and the logs are empty, that is the finding — report it as such rather
than filling the gap with the most likely story.

When the fix is a change, it is a change somebody else makes: name the page, say
what you would press, and offer to take them there.
