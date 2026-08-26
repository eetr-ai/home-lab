# The APIs this module needs. Declared so a fresh project reaches a working state
# from one apply, and never disabled on destroy: turning an API off is a
# project-wide action that would affect everything else using it.

resource "google_project_service" "artifact_registry" {
  project            = var.project_id
  service            = "artifactregistry.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "cloud_build" {
  project            = var.project_id
  service            = "cloudbuild.googleapis.com"
  disable_on_destroy = false
}
