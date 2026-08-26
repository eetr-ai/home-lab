# What this installation is

Load this when somebody asks what something *is*, where it lives, or why a thing
they expected to find is not there.

This is a home lab, built to be reproducible from its own repository:

- **libvirt virtual machines**, described in Terraform.
- **A kubeadm Kubernetes cluster** on those machines, brought up by Ansible.
- **Cluster platform services**, installed by one umbrella Helm chart
  (`home-lab-platform`) into the `platform-system` namespace.
- **This admin panel**, installed by a second chart (`home-lab-admin`) into the
  `admin` namespace.
- **PostgreSQL and MongoDB**, which do **not** run in the cluster.

## The one thing that surprises people

The databases run under Docker Compose on the virtualization host, not as pods.
There is no `postgres` pod to look at, no StatefulSet, no PVC behind them. When
somebody asks you to check on the database and means "is it up", the answer comes
from the panel's PostgreSQL and MongoDB routes — the API holds a connection — and
not from anything under `/api/kubernetes`.

Say so plainly when it comes up. "There is no pod for it" is a better answer than
an empty pod list.

## What runs in the cluster

In `platform-system`, from the platform chart:

| Piece | What it does |
| --- | --- |
| Traefik | The Gateway API implementation. Routing is **Gateway API**, never Ingress — there are no Ingress objects here at all, and a question about one is a question about an HTTPRoute. |
| cert-manager | Issues the TLS certificates the Gateway serves. |
| nfs-subdir-external-provisioner | Provides the `nfs-client` StorageClass, which is the cluster default. Every PVC here lands on it unless it says otherwise. |
| metrics-server | Serves CPU and memory usage. It does **not** serve disk usage; node disk is read from the kubelet, and only when the panel was granted it. |
| cloudflared | The tunnel that publishes what is published. |

In `admin`, from the admin chart: `admin-api`, `admin-web` (the panel a person is
looking at while they talk to you), and `admin-agent` — which is you.

## Where a route comes from

A workload is reachable from outside only if an HTTPRoute attaches it to the
`home-lab` Gateway in `platform-system`, and a namespace may only attach when it
carries the `home-lab.example/gateway-access=true` label. Two things to check, in
that order, when somebody says a hostname does not answer.

Administrative hostnames sit behind Cloudflare Access before their route is
enabled. That is a decision taken outside the cluster, so a route that looks right
and still refuses a visitor is usually Access and not Kubernetes.

## Storage

`nfs-client` is the default class and it is backed by an NFS export. Two
consequences worth knowing before you tell somebody anything about a volume:

- It is `ReadWriteMany`-capable, which is why a workload can be rescheduled onto
  another node and still find its data.
- Its reclaim policy deletes the backing directory when the claim goes. Deleting
  a PVC here destroys the data in it. Never suggest deleting one casually.

## What you are

You are an ordinary Octo app: a flow definition with an `ai-agent` block in it,
running on Octo's standalone runtime, deployed by the same chart as the rest of
the panel. Your memory and your workspace are a directory on an NFS volume, which
is why they survive a restart and why there is exactly one of you — the store is
held in your process and written back to that directory, so a second replica
would overwrite the first one's memory.

Mention any of that only if somebody asks. It is a fact, not a personality.
