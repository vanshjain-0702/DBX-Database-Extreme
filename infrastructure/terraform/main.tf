provider "aws" {
  region = var.aws_region
}

variable "aws_region" {
  default = "us-east-1"
}

variable "cluster_name" {
  default = "dbx-global-cluster"
}

# VPC Configuration
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"

  name = "${var.cluster_name}-vpc"
  cidr = "10.0.0.0/16"

  azs             = ["${var.aws_region}a", "${var.aws_region}b", "${var.aws_region}c"]
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]

  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true
}

# EKS Cluster
module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "19.16.0"

  cluster_name    = var.cluster_name
  cluster_version = "1.28"

  vpc_id                         = module.vpc.vpc_id
  subnet_ids                     = module.vpc.private_subnets
  cluster_endpoint_public_access = true

  eks_managed_node_groups = {
    # Control Plane (Orchestrator) needs stable IOPS
    orchestrator_nodes = {
      min_size     = 3
      max_size     = 5
      desired_size = 3

      instance_types = ["t3.medium"]
      capacity_type  = "ON_DEMAND"
    }

    # Data Plane (DBX Engines) needs massive RAM and NVMe SSDs
    data_nodes = {
      min_size     = 3
      max_size     = 10
      desired_size = 3

      # Using r6id.large as an example of a memory-optimized instance with local NVMe
      instance_types = ["r6id.large", "r6id.xlarge"]
      capacity_type  = "ON_DEMAND"

      # Taint the nodes so only DBX engines get scheduled here
      taints = {
        dedicated = {
          key    = "role"
          value  = "data-plane"
          effect = "NO_SCHEDULE"
        }
      }
    }
  }
}

# Output EKS kubeconfig update command
output "configure_kubectl" {
  description = "Configure kubectl: make sure you're logged into the correct AWS profile and run the following command to update your kubeconfig"
  value       = "aws eks --region ${var.aws_region} update-kubeconfig --name ${module.eks.cluster_name}"
}
