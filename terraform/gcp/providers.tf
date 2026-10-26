provider "google" {
  project = var.project_id
  region  = var.region
}

# The same provider, billing each request to this project explicitly. Used only
# where an API insists on it.
#
# Applied with a person's Application Default Credentials, a request names no
# quota project, and most APIs bill it to the project being changed. API Keys
# does not: it charges an unnamed request to Google's shared Cloud SDK project,
# where that API is off, and answers SERVICE_DISABLED for a service this module
# has just enabled.
#
# Not the default provider, on purpose. Billing every request here means every
# API this module touches — Cloud Resource Manager first — must be enabled in
# this project before the first plan can even read state, which a fresh project
# does not have. Scoped to the one resource that needs it, the only API it
# depends on is the one this module enables right before using it. And it lives
# here rather than in `gcloud auth application-default set-quota-project`, which
# would change the machine's setting for every other project on it.
provider "google" {
  alias                 = "billed"
  project               = var.project_id
  region                = var.region
  billing_project       = var.project_id
  user_project_override = true
}
