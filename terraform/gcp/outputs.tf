output "image_base" {
  description = "Registry prefix for the admin panel images and Helm chart."
  value       = local.image_base
}

output "build_service_account_email" {
  description = "Service account Cloud Build runs as."
  value       = google_service_account.build.email
}

output "puller_service_account_email" {
  description = "Read-only account behind the cluster's image pull secret."
  value       = google_service_account.puller.email
}
