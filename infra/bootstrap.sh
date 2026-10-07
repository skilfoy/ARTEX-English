#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl git openssl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
. /etc/os-release
printf 'deb [arch=%s signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu %s stable\n' "$(dpkg --print-architecture)" "$VERSION_CODENAME" > /etc/apt/sources.list.d/docker.list
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker

swap_mb=__SWAP_MB__
if [ "${swap_mb}" -gt 0 ]; then
  if ! fallocate -l "${swap_mb}M" /swapfile; then
    dd if=/dev/zero of=/swapfile bs=1M count="${swap_mb}"
  fi
  chmod 0600 /swapfile
  mkswap /swapfile
  swapon /swapfile
  grep -q '^/swapfile ' /etc/fstab || printf '/swapfile none swap sw 0 0\n' >> /etc/fstab
fi

install -d -m 0750 /opt/artex
git clone --depth 1 --branch '__GIT_REF__' '__REPO_URL__' /opt/artex/source
cd /opt/artex/source
password="$(openssl rand -hex 32)"
printf 'POSTGRES_PASSWORD=%s\nAPP_DOMAIN=%s\nNODE_HEAP_MB=%s\n' "$password" '__APP_DOMAIN__' '__NODE_HEAP_MB__' > .env
chmod 0600 .env
install -d -m 0750 data
echo "ARTEX build settings: swap=${swap_mb}MB node_heap=__NODE_HEAP_MB__MB"
docker compose -f docker-compose.cloud.yml up -d --build
