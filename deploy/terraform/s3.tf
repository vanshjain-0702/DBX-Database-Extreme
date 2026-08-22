# S3 Bucket for DBX Backups
resource "aws_s3_bucket" "dbx_backups" {
  bucket_prefix = "dbx-backups-"

  tags = {
    Name        = "DBX Enterprise Backups"
    Environment = "Production"
  }
}

resource "aws_s3_bucket_versioning" "dbx_backups_versioning" {
  bucket = aws_s3_bucket.dbx_backups.id
  versioning_configuration {
    status = "Enabled"
  }
}

# IAM Role for Orchestrator to access S3
resource "aws_iam_role" "dbx_orchestrator_s3_role" {
  name = "dbx_orchestrator_s3_access"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRoleWithWebIdentity"
        Effect = "Allow"
        Principal = {
          Federated = module.eks.oidc_provider_arn
        }
        Condition = {
          StringEquals = {
            "${module.eks.oidc_provider}:sub": "system:serviceaccount:default:dbx-orchestrator-sa"
          }
        }
      }
    ]
  })
}

resource "aws_iam_policy" "s3_backup_policy" {
  name        = "dbx_s3_backup_policy"
  description = "Allow DBX Orchestrator to upload backups to S3"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "s3:PutObject",
          "s3:GetObject",
          "s3:ListBucket"
        ]
        Effect   = "Allow"
        Resource = [
          aws_s3_bucket.dbx_backups.arn,
          "${aws_s3_bucket.dbx_backups.arn}/*"
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "attach_s3_policy" {
  role       = aws_iam_role.dbx_orchestrator_s3_role.name
  policy_arn = aws_iam_policy.s3_backup_policy.arn
}

output "backup_bucket_name" {
  value = aws_s3_bucket.dbx_backups.id
}
