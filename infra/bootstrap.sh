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

install -d -m 0750 /opt/artex
git clone --depth 1 --branch '__GIT_REF__' '__REPO_URL__' /opt/artex/source
cd /opt/artex/source
password="$(openssl rand -hex 32)"
printf 'POSTGRES_PASSWORD=%s\nAPP_DOMAIN=%s\n' "$password" '__APP_DOMAIN__' > .env
chmod 0600 .env
install -d -m 0750 data
docker compose -f docker-compose.cloud.yml up -d --build
