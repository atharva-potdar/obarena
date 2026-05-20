# ─── IAM Role for k0s Nodes ───────────────────────────────────────────────────
# Raw EC2 instances have zero AWS permissions by default. Without this, workers
# get 403 Forbidden on ECR pulls, and the EBS CSI driver can't create/attach
# volumes. EKS silently handled this; with k0s we must do it ourselves.

resource "aws_iam_role" "k0s_node" {
  name = "${var.cluster_name}-node-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
    }]
  })
}

# ECR read-only — allows k0s containerd to pull images from private ECR repos
resource "aws_iam_role_policy_attachment" "ecr_read" {
  role       = aws_iam_role.k0s_node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

# EBS CSI driver — allows volume create/attach/detach for gp3 persistent storage
resource "aws_iam_role_policy_attachment" "ebs_csi" {
  role       = aws_iam_role.k0s_node.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"
}

# SSM access — allows SSH-free remote management via AWS Systems Manager
resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.k0s_node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "k0s_node" {
  name = "${var.cluster_name}-node-profile"
  role = aws_iam_role.k0s_node.name
}

# ─── SSH Key Pair ─────────────────────────────────────────────────────────────

resource "aws_key_pair" "k0s" {
  key_name   = "${var.cluster_name}-k0s-key"
  public_key = var.ssh_public_key
}

# ─── Security Group ──────────────────────────────────────────────────────────

resource "aws_security_group" "k0s" {
  name        = "${var.cluster_name}-k0s-sg"
  description = "k0s cluster security group"
  vpc_id      = aws_vpc.main.id

  # SSH from admin
  ingress {
    description = "SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.admin_cidr]
  }

  # k0s API server
  ingress {
    description = "k0s API"
    from_port   = 6443
    to_port     = 6443
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.main.cidr_block]
  }

  # k0s join protocol (konnectivity, etcd, kubelet, VXLAN)
  ingress {
    description = "k0s internal"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    self        = true
  }

  # All egress
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.cluster_name}-k0s-sg"
  }
}

# ─── Controller Node ─────────────────────────────────────────────────────────

resource "aws_instance" "controller" {
  ami                    = var.ubuntu_ami
  instance_type          = var.controller_instance_type
  key_name               = aws_key_pair.k0s.key_name
  iam_instance_profile   = aws_iam_instance_profile.k0s_node.name
  vpc_security_group_ids = [aws_security_group.k0s.id]
  subnet_id              = aws_subnet.private[0].id

  # Allow containers to reach IMDS for IAM credentials (EBS CSI controller
  # may be scheduled here). Default hop limit of 1 blocks container access.
  metadata_options {
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
    http_endpoint               = "enabled"
  }

  root_block_device {
    volume_size = 50
    volume_type = "gp3"
  }

  tags = {
    Name = "${var.cluster_name}-controller"
    Role = "controller"
  }
}

# ─── Platform Workers ────────────────────────────────────────────────────────

resource "aws_instance" "platform" {
  count                  = var.platform_count
  ami                    = var.ubuntu_ami
  instance_type          = var.platform_instance_type
  key_name               = aws_key_pair.k0s.key_name
  iam_instance_profile   = aws_iam_instance_profile.k0s_node.name
  vpc_security_group_ids = [aws_security_group.k0s.id]
  subnet_id              = aws_subnet.private[count.index % length(aws_subnet.private)].id

  metadata_options {
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
    http_endpoint               = "enabled"
  }

  root_block_device {
    volume_size = 80
    volume_type = "gp3"
  }

  tags = {
    Name = "${var.cluster_name}-platform-${count.index}"
    Role = "platform"
  }
}

# ─── Sandbox Workers ─────────────────────────────────────────────────────────
# c5.4xlarge with SMT disabled = 8 real physical cores, no hyperthreading.
# This eliminates cache-thrashing from shared silicon — cores 2-7 are
# physically isolated, not just logically isolated via isolcpus.

resource "aws_instance" "sandbox" {
  count                  = var.sandbox_count
  ami                    = var.ubuntu_ami
  instance_type          = var.sandbox_instance_type
  key_name               = aws_key_pair.k0s.key_name
  iam_instance_profile   = aws_iam_instance_profile.k0s_node.name
  vpc_security_group_ids = [aws_security_group.k0s.id]
  subnet_id              = aws_subnet.private[count.index % length(aws_subnet.private)].id

  # Disable Hyperthreading: 8 physical cores, 1 thread each = 8 real cores.
  # Without this, c5.4xlarge gives 16 vCPUs on 8 physical cores (SMT),
  # and isolcpus would pin to shared silicon.
  cpu_options {
    core_count       = 8
    threads_per_core = 1
  }

  metadata_options {
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
    http_endpoint               = "enabled"
  }

  root_block_device {
    volume_size = 80
    volume_type = "gp3"
  }

  tags = {
    Name = "${var.cluster_name}-sandbox-${count.index}"
    Role = "sandbox"
  }
}
