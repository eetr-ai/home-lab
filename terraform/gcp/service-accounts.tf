# Two accounts, because pushing and pulling are different jobs with different
# blast radii. The build account writes; the cluster's account only reads, and its
# key lives in a Kubernetes Secret where a leak must not be able to overwrite a
# published image.
#
# Both roles are bound to the repository rather than the project, so neither
# account gains any standing in a registry it has no business touching.

resource "google_service_account" "build" {
  project      = var.project_id
  account_id   = "home-lab-build"
  display_name = "home-lab Cloud Build"
  description  = "Builds and publishes the admin panel images and Helm chart."
}

resource "google_artifact_registry_repository_iam_member" "build_writer" {
  project    = var.project_id
  location   = google_artifact_registry_repository.home_lab.location
  repository = google_artifact_registry_repository.home_lab.name
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.build.email}"
}

# A build running as a user-specified service account has to be able to write its
# own logs, and Cloud Build refuses to start without a log destination it can
# reach. This is project-scoped because log buckets are.
resource "google_project_iam_member" "build_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.build.email}"
}

# `gcloud builds submit` uploads the source to Cloud Build's staging bucket and the
# build then reads it back. A build running as its own service account has no
# standing there by default, so without this the manual submit — the documented way
# to exercise the pipeline without cutting a release — fails on a 403 for an object
# it just uploaded. Trigger-fired builds fetch their source from GitHub instead and
# do not depend on this.
#
# Scoped to that one bucket. Cloud Build creates it on first use, so on a brand new
# project run one build before applying this, or create the bucket first.
resource "google_storage_bucket_iam_member" "build_source_reader" {
  bucket = "${var.project_id}_cloudbuild"
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.build.email}"
}

resource "google_service_account" "puller" {
  project      = var.project_id
  account_id   = "home-lab-puller"
  display_name = "home-lab image puller"
  description  = "Read-only registry access for the cluster's image pull secret."
}

resource "google_artifact_registry_repository_iam_member" "puller_reader" {
  project    = var.project_id
  location   = google_artifact_registry_repository.home_lab.location
  repository = google_artifact_registry_repository.home_lab.name
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${google_service_account.puller.email}"
}

# Cloud Build impersonates the build account to run a triggered build, so its
# service agent needs permission to act as that account. Without this the trigger
# creates and then fails on its first run with a permission error that names
# neither account.
data "google_project" "this" {
  project_id = var.project_id
}

resource "google_service_account_iam_member" "cloud_build_agent_can_impersonate" {
  service_account_id = google_service_account.build.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:service-${data.google_project.this.number}@gcp-sa-cloudbuild.iam.gserviceaccount.com"

  depends_on = [google_project_service.cloud_build]
}
