terraform {
  backend "s3" {
    bucket         = "obarena-tf-state"
    key            = "platform/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "obarena-tf-locks"
    encrypt        = true
  }
}
