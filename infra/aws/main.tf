terraform {
  required_version = ">= 1.6.0"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 6.0" }
  }
}

provider "aws" {
  region = var.region
}

variable "region" {
  type    = string
  default = "us-east-1"
}
variable "name" {
  type    = string
  default = "artex-english"
}
variable "tier" {
  type    = string
  default = "standard"
  validation {
    condition     = contains(["free", "small", "standard", "work"], var.tier)
    error_message = "tier must be free, small, standard, or work."
  }
}
variable "instance_type" {
  type        = string
  default     = ""
  description = "Optional. Overrides the machine selected by tier. Disk size, swap, and the Node build heap still come from tier."
}
variable "ssh_public_key" { type = string }
variable "admin_cidr" { type = string }
variable "web_cidrs" {
  type    = list(string)
  default = []
}
variable "domain" {
  type    = string
  default = ""
  validation {
    condition     = var.domain == "" || can(regex("^[A-Za-z0-9.-]+$", var.domain))
    error_message = "domain must be a plain DNS hostname."
  }
}
variable "route53_zone_id" {
  type    = string
  default = ""
}
variable "git_ref" {
  type    = string
  default = "main"
  validation {
    condition     = can(regex("^[A-Za-z0-9._/-]+$", var.git_ref))
    error_message = "git_ref must contain only Git branch or tag characters."
  }
}

locals {
  tiers = {
    free     = { machine = "t3.micro", disk_gb = 30, node_heap_mb = 1536, swap_mb = 4096 }
    small    = { machine = "t3.small", disk_gb = 40, node_heap_mb = 1536, swap_mb = 2048 }
    standard = { machine = "t3.large", disk_gb = 100, node_heap_mb = 3072, swap_mb = 0 }
    work     = { machine = "t3.xlarge", disk_gb = 128, node_heap_mb = 3072, swap_mb = 0 }
  }
  spec          = local.tiers[var.tier]
  instance_type = var.instance_type != "" ? var.instance_type : local.spec.machine
  permitted_web_cidrs = length(var.web_cidrs) == 0 ? [var.admin_cidr] : var.web_cidrs
  bootstrap = replace(replace(replace(replace(replace(
    file("${path.module}/../bootstrap.sh"),
    "__REPO_URL__", "https://github.com/skilfoy/ARTEX-English.git"),
    "__GIT_REF__", var.git_ref),
    "__APP_DOMAIN__", var.domain == "" ? ":80" : var.domain),
    "__SWAP_MB__", tostring(local.spec.swap_mb)),
    "__NODE_HEAP_MB__", tostring(local.spec.node_heap_mb),
  )
}

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_vpc" "app" {
  cidr_block           = "10.42.0.0/16"
  enable_dns_hostnames = true
  tags                 = { Name = var.name }
}

resource "aws_subnet" "public" {
  vpc_id                  = aws_vpc.app.id
  cidr_block              = "10.42.1.0/24"
  map_public_ip_on_launch = true
  tags                    = { Name = "${var.name}-public" }
}

resource "aws_internet_gateway" "app" {
  vpc_id = aws_vpc.app.id
  tags   = { Name = var.name }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.app.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.app.id
  }
}

resource "aws_route_table_association" "public" {
  subnet_id      = aws_subnet.public.id
  route_table_id = aws_route_table.public.id
}

resource "aws_security_group" "app" {
  name_prefix = "${var.name}-"
  vpc_id      = aws_vpc.app.id
}

resource "aws_vpc_security_group_ingress_rule" "ssh" {
  security_group_id = aws_security_group.app.id
  cidr_ipv4         = var.admin_cidr
  from_port         = 22
  to_port           = 22
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "http" {
  for_each          = toset(local.permitted_web_cidrs)
  security_group_id = aws_security_group.app.id
  cidr_ipv4         = each.value
  from_port         = 80
  to_port           = 80
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "https" {
  for_each          = toset(local.permitted_web_cidrs)
  security_group_id = aws_security_group.app.id
  cidr_ipv4         = each.value
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_egress_rule" "all" {
  security_group_id = aws_security_group.app.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

resource "aws_key_pair" "admin" {
  key_name_prefix = "${var.name}-"
  public_key      = var.ssh_public_key
}

resource "aws_instance" "app" {
  ami                         = data.aws_ami.ubuntu.id
  instance_type               = local.instance_type
  subnet_id                   = aws_subnet.public.id
  vpc_security_group_ids      = [aws_security_group.app.id]
  key_name                    = aws_key_pair.admin.key_name
  associate_public_ip_address = true
  user_data                   = local.bootstrap
  root_block_device {
    volume_size = local.spec.disk_gb
    volume_type = "gp3"
    encrypted   = true
  }
  tags = { Name = var.name }
}

resource "aws_eip" "app" {
  domain     = "vpc"
  instance   = aws_instance.app.id
  depends_on = [aws_internet_gateway.app]
}

resource "aws_route53_record" "app" {
  count   = var.domain != "" && var.route53_zone_id != "" ? 1 : 0
  zone_id = var.route53_zone_id
  name    = var.domain
  type    = "A"
  ttl     = 60
  records = [aws_eip.app.public_ip]
}

output "public_ip" { value = aws_eip.app.public_ip }
output "app_url" { value = var.domain == "" ? "http://${aws_eip.app.public_ip}" : "https://${var.domain}" }
output "ssh_command" { value = "ssh ubuntu@${aws_eip.app.public_ip}" }
output "tier" { value = var.tier }
output "instance_type" { value = local.instance_type }
