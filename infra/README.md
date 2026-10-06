# Full-stack cloud deployment

The AWS, Google Cloud, and Azure configurations each provision an Ubuntu virtual machine, a static public address, network rules, and optional DNS. A startup script installs Docker, checks out this fork, and builds the Go backend with the complete frontend embedded. Docker Compose runs the application, PostgreSQL, and Caddy on the VM. The database is private to the Compose network.

The upstream author restricts use to study and locally isolated technical verification and prohibits testing online or networked systems. These configurations describe infrastructure capability and do not change those stated conditions. Review the [license and usage section](../README.md#license-and-use-conditions) before use.

## Choose a provider

Install Terraform and authenticate to the selected cloud account. Use one provider directory per deployment. Provide an SSH public key and the CIDR range allowed to reach the application during initial setup.

```bash
cd infra/aws
terraform init
terraform apply \
  -var='ssh_public_key=ssh-ed25519 AAAA... operator@example' \
  -var='admin_cidr=203.0.113.10/32'
```

For Google Cloud, run the same commands from `infra/gcp` and add `-var='project_id=YOUR_PROJECT_ID'`. For Azure, use `infra/azure` with an authenticated subscription. The AWS configuration accepts `region`, Google Cloud accepts `region` and `zone`, and Azure accepts `location`. Review `terraform plan` before applying.

The application is initially reachable over HTTP at the output address. Ingress defaults to `admin_cidr`. Complete the administrator setup, then configure an LLM provider in the application. The startup script generates a PostgreSQL password on the VM in `/opt/artex/source/.env`; Terraform state does not contain it.

## Domain and HTTPS

Set `domain` to a hostname you control. Caddy obtains a public certificate after DNS points to the VM and ports 80 and 443 are reachable. The provider configurations can create the DNS A record when the matching zone variable is set:

| Provider | Existing DNS zone variable |
| --- | --- |
| AWS | `route53_zone_id` |
| Google Cloud | `dns_managed_zone` |
| Azure | `dns_zone_name` and `dns_zone_resource_group` |

An external DNS provider also works if you create the A record yourself. To make the site public after administrator setup, set `web_cidrs=["0.0.0.0/0"]` in your Terraform variables and apply again. With a domain, the output URL uses HTTPS.

## Operations

The VM needs enough storage and memory to build the frontend and Go server. Default instance sizes provide two virtual CPUs, 8 GiB of memory, and a 100 GiB boot disk. A first build can take several minutes after `terraform apply` returns. Check progress with:

```bash
ssh ubuntu@PUBLIC_IP 'sudo cloud-init status --wait'
ssh ubuntu@PUBLIC_IP 'cd /opt/artex/source && sudo docker compose -f docker-compose.cloud.yml ps'
ssh ubuntu@PUBLIC_IP 'cd /opt/artex/source && sudo docker compose -f docker-compose.cloud.yml logs --tail=100 artex'
```

The application data directory, PostgreSQL Docker volume, and private `appstate` volume containing the JWT signing key reside on the VM boot disk. Back up all three before replacing or destroying the VM. Terraform state also contains infrastructure identifiers and startup configuration; store it in an access-controlled backend. To deploy a specific branch or tag, set `git_ref`. A change to startup data may replace a VM, so preserve a verified backup before applying such changes.

The cloud deployment runs this fork's source build. The separate [Vercel preview](https://artex-english.vercel.app) uses simulated frontend data.

## Provider references

- [AWS EC2 instance](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/instance)
- [Google Compute Engine instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance)
- [Azure Linux virtual machine](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/linux_virtual_machine)
- [Docker Compose health checks and startup order](https://docs.docker.com/compose/how-tos/startup-order/)
