data "aws_caller_identity" "current" {}

output "controller_ip" {
  description = "Private IP of the k0s controller node"
  value       = aws_instance.controller.private_ip
}

output "platform_ips" {
  description = "Private IPs of platform worker nodes"
  value       = aws_instance.platform[*].private_ip
}

output "sandbox_ips" {
  description = "Private IPs of sandbox worker nodes"
  value       = aws_instance.sandbox[*].private_ip
}

output "registry_url" {
  description = "Base URL for the ECR registry"
  value       = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.region}.amazonaws.com"
}

output "region" {
  description = "AWS region"
  value       = var.region
}

output "cluster_name" {
  description = "Name of the k0s cluster"
  value       = var.cluster_name
}
