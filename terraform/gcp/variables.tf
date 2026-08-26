# Values that identify an environment carry no default and are supplied through
# the ignored terraform.tfvars, matching the libvirt root module. Values that are
# structural facts of this repository do carry one, because there is exactly one
# right answer and repeating it in every checkout only creates a way to get it
# wrong.

variable "project_id" {
  description = "Google Cloud project that owns the registry and the build trigger."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{4,28}[a-z0-9]$", var.project_id))
    error_message = "project_id must be a valid Google Cloud project id."
  }
}

variable "region" {
  description = "Region holding the Artifact Registry repository and the builds."
  type        = string

  validation {
    condition     = can(regex("^[a-z]+-[a-z]+[0-9]$", var.region))
    error_message = "region must be a Google Cloud region such as us-west1."
  }
}

variable "registry_repository_id" {
  description = "Artifact Registry repository holding the admin images and the Helm chart."
  type        = string
  default     = "home-lab"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.registry_repository_id))
    error_message = "registry_repository_id must contain lowercase letters, numbers, and hyphens."
  }
}

variable "github_owner" {
  description = "GitHub account or organization owning the repository the trigger watches."
  type        = string
  default     = "eetr-ai"
}

variable "github_repo" {
  description = "GitHub repository the trigger watches."
  type        = string
  default     = "home-lab"
}

variable "release_tag_pattern" {
  description = "Regular expression matching the tags that publish the admin panel."
  type        = string
  default     = "^admin-v.*$"

  validation {
    condition     = startswith(var.release_tag_pattern, "^")
    error_message = "release_tag_pattern must be anchored so it cannot match a tag it was not meant to."
  }
}
