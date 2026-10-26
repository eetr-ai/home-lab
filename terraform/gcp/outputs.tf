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

output "picker_api_key" {
  description = "Browser key for the backups folder Picker: admin.api.backups.google.picker.apiKey."
  value       = google_apikeys_key.picker.key_string
  sensitive   = true
}

output "picker_app_id" {
  # The project number, not the id: it is what ties the folders the Picker hands
  # out to this project's OAuth client.
  description = "Project number, which the Picker takes as its app id: admin.api.backups.google.picker.appId."
  value       = data.google_project.this.number
}
