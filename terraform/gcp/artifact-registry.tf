# One Docker-format repository holds both the container images and the Helm
# chart. Artifact Registry serves OCI artifacts from a Docker repository, and
# `helm push` produces one, so a separate repository would only split the panel's
# release across two places that have to be kept at the same version.

resource "google_artifact_registry_repository" "home_lab" {
  project       = var.project_id
  location      = var.region
  repository_id = var.registry_repository_id
  description   = "home-lab admin panel images and Helm chart"
  format        = "DOCKER"
  mode          = "STANDARD_REPOSITORY"

  depends_on = [google_project_service.artifact_registry]
}
