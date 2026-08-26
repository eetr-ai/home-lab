# A first-generation GitHub trigger, matching the one that already publishes octo
# from this project. It requires the Cloud Build GitHub App to be installed on the
# repository's owner, which is a console step Terraform cannot perform — see
# README.md. Until it is done, this resource fails to create while everything else
# in this module applies cleanly.

locals {
  image_base = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.home_lab.repository_id}"
}

resource "google_cloudbuild_trigger" "admin_publish" {
  project     = var.project_id
  location    = "global"
  name        = "home-lab-admin-publish"
  description = "Build and push the admin panel images and Helm chart to Artifact Registry on release tags."

  filename        = "cloudbuild.yaml"
  service_account = google_service_account.build.id

  github {
    owner = var.github_owner
    name  = var.github_repo

    push {
      tag = var.release_tag_pattern
    }
  }

  substitutions = {
    _IMAGE_BASE = local.image_base
    _REGION     = var.region
    # Resolved by Cloud Build from the tag that fired the trigger, so a build is
    # always addressed by the version it came from and never by a moving name.
    _TAG = "$TAG_NAME"
  }

  depends_on = [
    google_project_service.cloud_build,
    google_service_account_iam_member.cloud_build_agent_can_impersonate,
  ]
}
