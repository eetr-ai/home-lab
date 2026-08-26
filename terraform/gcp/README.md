# Google Cloud build and registry

This root module owns everything the admin panel's release path needs outside the
cluster: the Artifact Registry repository the images and Helm chart are published
to, the two service accounts that write and read it, and the Cloud Build trigger
that fires on a release tag.

It is separate from the [libvirt module](../README.md) because the two share
nothing. Keeping them apart means a registry change cannot plan against virtual
machine state, and a failed apply in one leaves the other untouched.

## Manual prerequisite

**The Cloud Build GitHub App must be installed on the repository's owner before
the trigger can be created.** Terraform has no way to perform the OAuth handshake
that authorizes it, so the first apply fails on
`google_cloudbuild_trigger.admin_publish` until this is done. Everything else in
the module applies cleanly in the meantime.

Install it from the Cloud Build console — Triggers → Connect repository — and
choose the repository this module names in `github_owner` / `github_repo`.

## Apply

```bash
cp terraform/gcp/terraform.tfvars.example terraform/gcp/terraform.tfvars
# replace project_id and region

terraform -chdir=terraform/gcp init
terraform -chdir=terraform/gcp plan
terraform -chdir=terraform/gcp apply
```

`terraform.tfvars` is ignored, like every other one in this repository.

## What it creates

| Resource | Purpose |
| --- | --- |
| `google_artifact_registry_repository.home_lab` | One Docker repository holding both images and the OCI Helm chart |
| `google_service_account.build` | The identity Cloud Build runs as; writes to the repository and its own logs |
| `google_service_account.puller` | Read-only; the identity behind the cluster's image pull secret |
| `google_cloudbuild_trigger.admin_publish` | Runs `cloudbuild.yaml` on tags matching `^admin-v.*$` |

Both registry roles are bound to the repository rather than the project, so
neither account has any standing in a registry it should not touch. The puller's
key ends up in a Kubernetes Secret, and a read-only account there cannot be used
to overwrite a published image.

## Releasing

Nothing here is run to publish a release. release-please opens a release pull
request from the Conventional Commit history; merging it tags `admin-vX.Y.Z`, and
the tag fires this trigger. To publish without a tag — to check a pipeline change,
say — submit the build directly:

```bash
gcloud builds submit --config cloudbuild.yaml --substitutions=_TAG=dev-local .
```

## State and secrets

State is local, held on the operator's encrypted laptop, and is ignored along with
`terraform.tfvars`. As with the libvirt module, migrate to a remote backend with
encryption and locking before adding a second operator or running Terraform in CI.

The service account **keys** are not managed here. Terraform would write a private
key into state, and this repository's state is a local file. Create the puller's
key by hand when the cluster needs it — see
[the admin chart](../../charts/admin/README.md).
