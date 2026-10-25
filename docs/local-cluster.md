# A local cluster

`task dev` runs the panel, the API and the assistant as plain containers, which is
quick, but it has no cluster. The Kubernetes and Helm sections answer 404 there,
and a backup schedule, which is a CronJob, has nowhere to run.

`task cluster:deploy` fills that gap. It runs the same chart on a throwaway
[k3d](https://k3d.io) cluster on this machine. PostgreSQL and MongoDB run as
containers beside the cluster, the way the lab runs them beside its own.

```bash
task cluster:deploy   # create or start everything, build, install, forward :3000
task cluster:stop     # stop it all, keep the data
task cluster:delete   # remove it all, data included
```

## Before the first deploy

- **Tools:** Docker, `k3d`, `kubectl`, `helm` and `openssl`.
- **`admin/.env`:** the file `task dev` uses. The local cluster signs in with the
  same eetr-auth client, so its redirect URIs must include
  `http://localhost:3000/api/auth/callback/eetr`.
- **Optional, for backups:** the `GOOGLE_*` variables in `admin/.env.example`.
  Without them the Backups section says Drive is not set up, and the rest works.

## What a deploy does

1. Creates the cluster `home-lab-local`, or starts it if it already exists. It
   has no load balancer and no Traefik, and its kubeconfig goes to the state
   directory, so `~/.kube/config` is never touched.
2. Starts the databases from `local/compose.yaml` on the cluster's Docker
   network.
   - Pods can't resolve container names, so each database becomes a
     selectorless Service in `local-databases`. Its EndpointSlice holds the
     container's address, read at deploy time.
   - Neither database is published on the host.
3. Builds `admin-api` and `admin-web` from this checkout under a fresh
   `local-<time>` tag, then imports them, along with the backup jobs' dump
   images, straight into the cluster.
4. Creates the Secrets from `admin/.env` and the database passwords it
   generated on the first deploy. It then installs the chart with
   `local/values.yaml`:
   - one replica each
   - no Redis, agent or console
   - Helm deployments switched on, with the panel's database on the local
     PostgreSQL
5. Forwards `http://localhost:3000` to the panel in the background.
   `task cluster:stop` ends it.

The generated passwords, the kubeconfig and the port-forward's pid and log live
in `~/.local/state/home-lab/local-cluster`, outside the repository. There is one
per machine, not one per checkout, because the cluster and the database volumes
are per machine too. Set `HOME_LAB_LOCAL_STATE` to put them somewhere else.

To use kubectl on the cluster:

```bash
export KUBECONFIG="$HOME/.local/state/home-lab/local-cluster/kubeconfig"
```

Run `task cluster:deploy` again after changing the code. It rebuilds and
upgrades in place, and the data stays.
