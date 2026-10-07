# Full-stack cloud deployment

These configurations run the real English application in the cloud: the Go server, PostgreSQL, and Caddy on one Ubuntu virtual machine. They are not the Vercel preview. [artex-english.vercel.app](https://artex-english.vercel.app) is a frontend-only build with `NEXT_PUBLIC_MOCK=1` and never starts Postgres or the agent runtime.

Each provider directory is a complete, separate stack. Pick one. Do not apply two of them for the same deployment.

| Directory | Machine | Standard tier | Default place |
| --- | --- | --- | --- |
| [aws/](aws/main.tf) | EC2 | `t3.large` (2 vCPU, 8 GiB) | `us-east-1` |
| [gcp/](gcp/main.tf) | Compute Engine | `e2-standard-2` (2 vCPU, 8 GiB) | `us-central1-a` |
| [azure/](azure/main.tf) | Linux VM | `Standard_D2s_v5` (2 vCPU, 8 GiB) | `eastus` |

`tier` defaults to `standard`. Set `free`, `small`, or `work` when that is the wrong size. The same four names exist in every provider. An empty machine-size variable means "use the tier." Setting `instance_type`, `machine_type`, or `vm_size` replaces only the machine. Disk, swap, and the Node build heap still come from `tier`.

All three create a new network, a static public address, a disk sized for the tier, and firewall rules. Docker Compose on the VM then starts:

- `postgres`, reachable only on the Compose network
- `artex`, the Go process with the English UI embedded, also only on the Compose network
- `caddy`, published on ports 80 and 443 and reverse-proxying to `artex:8787`

Ports 8787 and 8788 are not open on the public address. The traffic-recording proxy stays inside the Compose network, same as the local Compose file binding it to localhost.

The upstream author restricts use to study and locally isolated technical verification and prohibits testing online or networked systems. These configurations describe infrastructure and do not change those stated conditions. Review the [license and usage section](../README.md#license-and-use-conditions) before provisioning or using the software.

## Choose a tier

Pick the tier for the job, not the largest one that will start. The application image compiles the Next.js UI, the Go server, and a Chromium install on the VM. That build is what sets the minimum size. The Dockerfile asks Node for 3 GiB of heap on `standard` and `work`. `free` and `small` lower that to 1.5 GiB, add swap, and compile the frontend before Go starts. They can still run out of memory or disk. If the first boot must finish unattended, use `standard`.

| Tier | Use it for | AWS | Google Cloud | Azure | Disk | Swap |
| --- | --- | --- | --- | --- | --- | --- |
| `free` | The provider's smallest published free shape | `t3.micro`, 1 GiB | `e2-micro`, 1 GiB, standard disk | `Standard_B1s`, 1 GiB | 30 GiB, or 32 GiB on Azure | 4 GiB |
| `small` | A cheap always-on UI when a failed build can be retried | `t3.small`, 2 GiB | `e2-small`, 2 GiB | `Standard_B2s`, 2 vCPU, 4 GiB | 40 GiB, or 64 GiB on Azure | 2 GiB |
| `standard` | The default. The VM builds the image itself | `t3.large`, 8 GiB | `e2-standard-2`, 8 GiB | `Standard_D2s_v5`, 8 GiB | 100 GiB, or 128 GiB on Azure | none |
| `work` | Several agents at the same time | `t3.xlarge`, 16 GiB | `e2-standard-4`, 16 GiB | `Standard_D4s_v5`, 16 GiB | 128 GiB | none |

```hcl
tier = "free"      # or small, standard, work
```

What "free" actually costs:

- **Google Cloud** is the one that can stay at $0. Always Free is one non-preemptible `e2-micro` in `us-west1`, `us-central1`, or `us-east1`, 30 GiB of standard persistent disk, and 1 GiB of North American egress. The default zone `us-central1-a` qualifies. `tier = "free"` in any other region fails the plan. A balanced disk, a second VM, or a larger machine is billed. An external address attached to a running VM is included. An unused address is not.
- **AWS** is not an always-free server. Accounts created before 15 July 2025 get 750 hours a month of `t2.micro` or `t3.micro`, plus 30 GiB of `gp2` or `gp3`, for 12 months. Accounts created on or after that date get a credit balance, for 6 months or until the credits run out, and `t3.micro` is one of the sizes those credits can pay for. `tier = "free"` selects `t3.micro` and a 30 GiB encrypted `gp3` disk so it fits those allowances. Traffic, addresses beyond the allowance, and anything after the period are billed. Confirm the current terms on the [EC2 Free Tier page](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-free-tier-usage.html).
- **Azure** is not an entirely free server. A new account includes 750 hours a month of `Standard_B1s` for 12 months. `tier = "free"` uses that size and a 32 GiB Standard SSD, because B1s cannot take a Premium disk. A Standard static public IP is a separate meter and is commonly billed even while the VM hours are free. Confirm the current list on the [Azure free services page](https://learn.microsoft.com/en-us/azure/cost-management-billing/manage/create-free-services).

The Vercel preview remains the free way to click through the interface. It does not become this VM.

Changing `tier` after the first apply changes the disk size or type and usually replaces the VM. Back up `/opt/artex/source/data` and the Docker volumes first. See [Operations](#operations).

## What you need before apply

- Terraform 1.6 or newer.
- A cloud login for the provider you chose. See the provider sections below.
- An SSH public key. The private key stays on your machine. The VM user is `ubuntu`.
- Your current public IP in CIDR form, for example `203.0.113.10/32`. That value is `admin_cidr`. SSH is always limited to it. The web ports use it too until you set `web_cidrs`.
- Optional: a DNS hostname and, if this Terraform stack should create the record, an existing DNS zone in the same account.

Copy the example variable file for that provider and edit it. Do not commit a filled-in `terraform.tfvars`. It contains your key and your IP, and Terraform will also write `terraform.tfstate` beside it. Keep both private. The Postgres password is not in state. The startup script generates it on the VM.

```bash
cd infra/aws   # or infra/gcp, or infra/azure
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars
terraform init
terraform plan
terraform apply
```

`terraform apply` finishes when the VM exists. The first image build continues after that and often takes several minutes. Wait for it before treating the URL as down.

```bash
ssh ubuntu@PUBLIC_IP 'cloud-init status --wait'
ssh ubuntu@PUBLIC_IP 'cd /opt/artex/source && sudo docker compose -f docker-compose.cloud.yml ps'
```

Google Cloud runs the same script through the guest agent rather than cloud-init. If `cloud-init` is not the log you need, use:

```bash
ssh ubuntu@PUBLIC_IP 'sudo journalctl -u google-startup-scripts.service --no-pager'
```

On AWS and Azure the script log is `/var/log/cloud-init-output.log`.

## Shared options

These variables exist in every provider. Names that differ are in the provider sections.

| Variable | Required | Default | Effect |
| --- | --- | --- | --- |
| `ssh_public_key` | yes | none | OpenSSH public key for the `ubuntu` user. A single line, such as `ssh-ed25519 AAAA... operator@example`. |
| `admin_cidr` | yes | none | IPv4 CIDR allowed to use SSH. Also the web allow-list when `web_cidrs` is empty. Use a `/32` for one address. |
| `web_cidrs` | no | `[]` | IPv4 CIDRs allowed to open ports 80 and 443. Empty means `admin_cidr` only. `["0.0.0.0/0"]` makes the site reachable from the internet. |
| `domain` | no | empty | Hostname Caddy will serve. Empty serves plain HTTP on the public IP. A value switches the printed URL to `https://` that name. |
| `git_ref` | no | `main` | Branch or tag cloned onto the VM. Must match `[A-Za-z0-9._/-]+`. A raw commit SHA does not work: the script uses `git clone --branch`. |
| `name` | no | `artex-english` | Name prefix for the VM and network resources. Changing it after apply creates a second stack only if you also change the state; normally treat it as fixed. |
| `tier` | no | `standard` | `free`, `small`, `standard`, or `work`. See [Choose a tier](#choose-a-tier). |

`git_ref` is read at first boot only. Changing it later does not, by itself, pull new code. See [Updating the application](#updating-the-application).

## AWS

Authenticate with the usual AWS chain: a profile (`AWS_PROFILE`), environment keys, or an IAM role. The stack uses provider `hashicorp/aws` `~> 6.0` and looks up the current Ubuntu 24.04 LTS AMI from Canonical (`099720109477`).

Extra variables:

| Variable | Default | Effect |
| --- | --- | --- |
| `region` | `us-east-1` | Region for the VPC, instance, and address. |
| `instance_type` | empty | Optional machine override, such as `t3.large`. Empty uses the tier. |
| `route53_zone_id` | empty | When this and `domain` are both set, Terraform creates an A record in that existing public hosted zone. |

```bash
cd infra/aws
terraform apply \
  -var='ssh_public_key=ssh-ed25519 AAAA... operator@example' \
  -var='admin_cidr=203.0.113.10/32'
```

The example file [aws/terraform.tfvars.example](aws/terraform.tfvars.example) lists the same options. The stack creates a dedicated VPC (`10.42.0.0/16`), one public subnet, an internet gateway, a security group, an SSH key pair, an encrypted `gp3` root disk sized by the tier, and an Elastic IP. There is no NAT gateway and no load balancer. The instance itself has the public address.

## Google Cloud

Install the `gcloud` CLI and set Application Default Credentials. The provider does not prompt for a project in the browser.

```bash
gcloud auth application-default login
gcloud config set project YOUR_PROJECT_ID
```

The stack uses provider `hashicorp/google` `~> 7.0`. Compute Engine and Cloud DNS APIs must be enabled if you use the DNS record. The image is `ubuntu-os-cloud/ubuntu-2404-lts-amd64`.

Extra variables:

| Variable | Default | Effect |
| --- | --- | --- |
| `project_id` | none, required | Existing project that will own the VM. |
| `region` | `us-central1` | Region for the subnet and the static address. |
| `zone` | `us-central1-a` | Zone for the VM. It must belong to `region`. |
| `machine_type` | empty | Optional machine override, such as `e2-standard-2`. Empty uses the tier. `tier = "free"` ignores a larger type only when this stays empty, and it must stay in `us-west1`, `us-central1`, or `us-east1`. |
| `dns_managed_zone` | empty | When this and `domain` are both set, Terraform creates an A record in that existing Cloud DNS zone. The record name is `domain` with a trailing dot. |

```bash
cd infra/gcp
terraform apply \
  -var='project_id=YOUR_PROJECT_ID' \
  -var='ssh_public_key=ssh-ed25519 AAAA... operator@example' \
  -var='admin_cidr=203.0.113.10/32'
```

The network is custom-mode `10.43.1.0/24` with no auto subnets. The VM has an external static address and a disk sized by the tier. `free` uses `pd-standard`, which is the Always Free disk type. The other tiers use `pd-balanced`. Google encrypts persistent disks. Firewall tags match the instance name.

## Azure

Sign in and select the subscription that should own the new resource group.

```bash
az login
az account set --subscription YOUR_SUBSCRIPTION_ID
```

The stack uses provider `hashicorp/azurerm` `~> 4.0`. It creates a new resource group named `<name>-rg`. It does not reuse an existing group.

Extra variables:

| Variable | Default | Effect |
| --- | --- | --- |
| `location` | `eastus` | Azure region. |
| `vm_size` | empty | Optional size override, such as `Standard_D2s_v5`. Empty uses the tier. Confirm the size exists in `location`. |
| `dns_zone_name` | empty | Existing DNS zone name. Used only together with `domain` and `dns_zone_resource_group`. |
| `dns_zone_resource_group` | empty | Resource group that already holds that DNS zone. It can differ from the group this stack creates. |

```bash
cd infra/azure
terraform apply \
  -var='ssh_public_key=ssh-ed25519 AAAA... operator@example' \
  -var='admin_cidr=203.0.113.10/32'
```

The network is `10.44.0.0/16` with subnet `10.44.1.0/24`, a Standard static public IP, and a network security group. `free` and `small` use a Standard SSD because the B-series sizes do not accept a Premium disk. `standard` and `work` use `Premium_LRS`. The image is Canonical Ubuntu Server 24.04 LTS. Password login is disabled.

If `domain` equals `dns_zone_name`, the A record is created at the zone apex. Otherwise `domain` must be a name inside that zone, and Terraform strips the zone suffix to form the relative record.

## After the first boot

Open the `app_url` output. With no domain that is `http://PUBLIC_IP`. The first request should show administrator setup. Set the password there. There is no default password.

Then open LLM settings in the application and save a provider key. The cloud Compose file does not pass `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, or `ARTEX_LLM_*` into the container. The local [docker-compose.yml](../docker-compose.yml) does. On this path the key is stored by the application in Postgres, which is the copy that survives a container recreate. Until a model is saved, the UI and API run, and agent runs do not.

The generated database password is only in `/opt/artex/source/.env` on the VM, mode `0600`. Terraform never sees it. You need it only for direct database administration. Application login is the password you set in the setup page.

## Domain and HTTPS

Leave `domain` empty to stay on HTTP at the public IP. That is the right first deployment while `web_cidrs` is still your own IP.

To serve a name:

1. Set `domain` to the full hostname, for example `artex.example.com`.
2. Either let Terraform create the A record, or create it yourself at whatever DNS host you use. The record must point at the `public_ip` output.
3. Apply again.

Caddy requests a publicly trusted certificate over HTTP-01. Let's Encrypt has to reach port 80 on that hostname from the internet. A firewall that only allows `admin_cidr` will fail issuance. Set `web_cidrs=["0.0.0.0/0"]` before expecting HTTPS, or keep using HTTP by IP. This stack has no DNS-01 challenge and no private certificate option.

| Provider | Zone variable | What you must already have |
| --- | --- | --- |
| AWS | `route53_zone_id` | A public hosted zone ID |
| Google Cloud | `dns_managed_zone` | A Cloud DNS managed zone name |
| Azure | `dns_zone_name` and `dns_zone_resource_group` | An Azure DNS zone |

An external DNS provider is the other option. Leave the zone variables empty and create the A record yourself. Terraform will not try to manage it.

## Who can connect

| Port | Default source | How to change it |
| --- | --- | --- |
| 22 | `admin_cidr` only | Change `admin_cidr` and apply. There is no separate SSH list. |
| 80, 443 | `admin_cidr` until `web_cidrs` is set | Set `web_cidrs` to one or more CIDRs. This replaces the default. It does not add to it. Include your own IP if you still want access. |
| 5432, 8787, 8788 | not open | Not published. Do not add them to make the demo "easier". Postgres and the recording proxy are internal. |

`web_cidrs=["0.0.0.0/0"]` exposes the finished UI and API to the internet. SSH stays on `admin_cidr`. Do that only after administrator setup, and only if a public site is actually required.

## Operations

Useful commands, once `PUBLIC_IP` is the `public_ip` output:

```bash
ssh ubuntu@PUBLIC_IP
ssh ubuntu@PUBLIC_IP 'cd /opt/artex/source && sudo docker compose -f docker-compose.cloud.yml ps'
ssh ubuntu@PUBLIC_IP 'cd /opt/artex/source && sudo docker compose -f docker-compose.cloud.yml logs --tail=100 artex'
ssh ubuntu@PUBLIC_IP 'cd /opt/artex/source && sudo docker compose -f docker-compose.cloud.yml logs --tail=100 caddy'
```

The application data directory is `/opt/artex/source/data`. Postgres lives in the Compose volume `pgdata`. The JWT signing key lives in the `appstate` volume. All three are on the VM boot disk. Copy them off the VM before you replace or destroy it. Terraform state holds resource IDs, the public IP, and the startup script. Store state in a private backend if more than one person will apply this stack. The scripts do not configure a remote backend.

### Updating the application

The VM cloned the repo once. To run a newer `main`, SSH in and rebuild:

```bash
cd /opt/artex/source
sudo git fetch --depth 1 origin main
sudo git checkout FETCH_HEAD
sudo docker compose -f docker-compose.cloud.yml up -d --build
```

Keep `.env` and the `data` directory. A rebuild does not remove the Docker volumes.

Changing `git_ref` in Terraform updates the startup script, but that script does not run again on an existing disk. Azure treats a `custom_data` change as a new VM, which would wipe the disk. Prefer the SSH update above.

### Destroy

From the same directory, with the same variable file:

```bash
terraform destroy
```

This deletes the VM, the disk, and the static address. Back up `data`, `pgdata`, and `appstate` first if you need them. DNS records created by Terraform are removed. Records you created by hand are not.

## What these stacks do not do

- They do not deploy the Vercel mock, and they do not turn the Vercel project into the full application.
- They do not make AWS or Azure a $0 deployment. Google Cloud `tier = "free"` is the Always Free shape, and only inside its published region, disk, and egress limits. See [Choose a tier](#choose-a-tier).
- They do not guarantee that `free` or `small` will finish the image build. Use `standard` when the first boot has to succeed on its own.
- They do not create a managed database, a second availability zone, backups, or autoscaling.
- They do not inject LLM API keys. Set those in the UI after setup.
- They do not publish the recording proxy.
- They do not store Terraform state remotely.
- They do not clone a private repository. This fork is public. A private remote would need a credential the script does not have.

## Files

| File | Role |
| --- | --- |
| [bootstrap.sh](bootstrap.sh) | First-boot script. Installs Docker, clones this fork, writes `.env`, and starts Compose. Terraform substitutes the repo URL, `git_ref`, and domain. |
| [Caddyfile](Caddyfile) | Reverse proxy from `$APP_DOMAIN` to `artex:8787`. |
| [../docker-compose.cloud.yml](../docker-compose.cloud.yml) | Postgres, the application image built from this repo, and Caddy. |
| [aws/main.tf](aws/main.tf), [gcp/main.tf](gcp/main.tf), [azure/main.tf](azure/main.tf) | One root module per provider. Variables, resources, and outputs are in that single file. |

## Provider references

- [AWS EC2 instance](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/instance)
- [Google Compute Engine instance](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance)
- [Azure Linux virtual machine](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/linux_virtual_machine)
- [Docker Compose health checks and startup order](https://docs.docker.com/compose/how-tos/startup-order/)
