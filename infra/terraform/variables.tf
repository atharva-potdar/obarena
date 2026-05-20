variable "region" {
  description = "AWS region"
  default     = "us-east-1"
}

variable "cluster_name" {
  description = "Name of the k0s cluster"
  default     = "obarena-platform"
}

variable "environment" {
  description = "Deployment environment: dev or prod"
  type        = string
  default     = "dev"

  validation {
    condition     = contains(["dev", "prod"], var.environment)
    error_message = "Environment must be 'dev' or 'prod'."
  }
}

# ─── SSH & Access ─────────────────────────────────────────────────────────────

variable "ssh_public_key" {
  description = "SSH public key for EC2 instance access"
  type        = string
}

variable "admin_cidr" {
  description = "CIDR block allowed to SSH into nodes (lock down for production)"
  type        = string
  default     = "0.0.0.0/0"
}

# ─── AMI ──────────────────────────────────────────────────────────────────────

variable "ubuntu_ami" {
  description = "Ubuntu 24.04 LTS AMI ID for the target region"
  type        = string
  default     = "ami-0a0e5d9c7acc336f1" # us-east-1, update per region
}

# ─── Controller ──────────────────────────────────────────────────────────────

variable "controller_instance_type" {
  description = "Instance type for the k0s controller node"
  default     = "t3.medium"
}

# ─── Platform Workers ────────────────────────────────────────────────────────

variable "platform_instance_type" {
  description = "Instance type for platform worker nodes"
  default     = "m5.xlarge"
}

variable "platform_count" {
  description = "Number of platform worker nodes"
  type        = number
  default     = 2
}

# ─── Sandbox Workers ─────────────────────────────────────────────────────────
# c5.4xlarge = 16 vCPUs on 8 physical cores. With cpu_options
# threads_per_core=1, we get 8 real unshared cores — no hyperthreading.

variable "sandbox_instance_type" {
  description = "Instance type for sandbox workers (needs dedicated physical cores for CPU pinning)"
  default     = "c5.4xlarge"
}

variable "sandbox_count" {
  description = "Number of sandbox worker nodes"
  type        = number
  default     = 2
}
