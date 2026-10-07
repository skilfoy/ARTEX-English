terraform {
  required_version = ">= 1.6.0"
  required_providers {
    azurerm = { source = "hashicorp/azurerm", version = "~> 4.0" }
  }
}

provider "azurerm" {
  features {}
}

variable "location" {
  type    = string
  default = "eastus"
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
variable "vm_size" {
  type        = string
  default     = ""
  description = "Optional. Overrides the size selected by tier. Disk size, swap, and the Node build heap still come from tier."
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
variable "dns_zone_name" {
  type    = string
  default = ""
}
variable "dns_zone_resource_group" {
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
    free     = { machine = "Standard_B1s", disk_gb = 32, disk_type = "StandardSSD_LRS", node_heap_mb = 1536, swap_mb = 4096 }
    small    = { machine = "Standard_B2s", disk_gb = 64, disk_type = "StandardSSD_LRS", node_heap_mb = 1536, swap_mb = 2048 }
    standard = { machine = "Standard_D2s_v5", disk_gb = 128, disk_type = "Premium_LRS", node_heap_mb = 3072, swap_mb = 0 }
    work     = { machine = "Standard_D4s_v5", disk_gb = 128, disk_type = "Premium_LRS", node_heap_mb = 3072, swap_mb = 0 }
  }
  spec                = local.tiers[var.tier]
  vm_size             = var.vm_size != "" ? var.vm_size : local.spec.machine
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

resource "azurerm_resource_group" "app" {
  name     = "${var.name}-rg"
  location = var.location
}

resource "azurerm_virtual_network" "app" {
  name                = "${var.name}-vnet"
  address_space       = ["10.44.0.0/16"]
  location            = azurerm_resource_group.app.location
  resource_group_name = azurerm_resource_group.app.name
}

resource "azurerm_subnet" "app" {
  name                 = "${var.name}-subnet"
  resource_group_name  = azurerm_resource_group.app.name
  virtual_network_name = azurerm_virtual_network.app.name
  address_prefixes     = ["10.44.1.0/24"]
}

resource "azurerm_public_ip" "app" {
  name                = "${var.name}-ip"
  location            = azurerm_resource_group.app.location
  resource_group_name = azurerm_resource_group.app.name
  allocation_method   = "Static"
  sku                 = "Standard"
}

resource "azurerm_network_security_group" "app" {
  name                = "${var.name}-nsg"
  location            = azurerm_resource_group.app.location
  resource_group_name = azurerm_resource_group.app.name
}

resource "azurerm_network_security_rule" "ssh" {
  name                        = "ssh"
  priority                    = 100
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "22"
  source_address_prefix       = var.admin_cidr
  destination_address_prefix  = "*"
  resource_group_name         = azurerm_resource_group.app.name
  network_security_group_name = azurerm_network_security_group.app.name
}

resource "azurerm_network_security_rule" "web" {
  name                        = "web"
  priority                    = 110
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_ranges     = ["80", "443"]
  source_address_prefixes     = local.permitted_web_cidrs
  destination_address_prefix  = "*"
  resource_group_name         = azurerm_resource_group.app.name
  network_security_group_name = azurerm_network_security_group.app.name
}

resource "azurerm_network_interface" "app" {
  name                = "${var.name}-nic"
  location            = azurerm_resource_group.app.location
  resource_group_name = azurerm_resource_group.app.name
  ip_configuration {
    name                          = "public"
    subnet_id                     = azurerm_subnet.app.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.app.id
  }
}

resource "azurerm_network_interface_security_group_association" "app" {
  network_interface_id      = azurerm_network_interface.app.id
  network_security_group_id = azurerm_network_security_group.app.id
}

resource "azurerm_linux_virtual_machine" "app" {
  name                            = var.name
  resource_group_name             = azurerm_resource_group.app.name
  location                        = azurerm_resource_group.app.location
  size                            = local.vm_size
  admin_username                  = "ubuntu"
  disable_password_authentication = true
  network_interface_ids           = [azurerm_network_interface.app.id]
  custom_data                     = base64encode(local.bootstrap)

  admin_ssh_key {
    username   = "ubuntu"
    public_key = var.ssh_public_key
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = local.spec.disk_type
    disk_size_gb         = local.spec.disk_gb
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "ubuntu-24_04-lts"
    sku       = "server"
    version   = "latest"
  }
}

resource "azurerm_dns_a_record" "app" {
  count               = var.domain != "" && var.dns_zone_name != "" && var.dns_zone_resource_group != "" ? 1 : 0
  name                = var.domain == var.dns_zone_name ? "@" : trimsuffix(var.domain, ".${var.dns_zone_name}")
  zone_name           = var.dns_zone_name
  resource_group_name = var.dns_zone_resource_group
  ttl                 = 60
  records             = [azurerm_public_ip.app.ip_address]
}

output "public_ip" { value = azurerm_public_ip.app.ip_address }
output "app_url" { value = var.domain == "" ? "http://${azurerm_public_ip.app.ip_address}" : "https://${var.domain}" }
output "ssh_command" { value = "ssh ubuntu@${azurerm_public_ip.app.ip_address}" }
output "tier" { value = var.tier }
output "vm_size" { value = local.vm_size }
