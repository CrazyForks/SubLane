#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'HELP'
Install SubLane from a published Docker image or Linux binary.

Usage: bash install.sh [--version VERSION] [--dir DIRECTORY] [--port PORT]
                       [--runtime docker|binary] [--proxy none|caddy|nginx]
                       [--domain DOMAIN] [--trusted-proxies CIDRS]
                       [--wait-timeout SECONDS] [--demo] [--non-interactive]

  --version VERSION  Published version, with or without v (default: latest stable,
                     or newest published prerelease when no stable release exists)
  --dir DIRECTORY    New installation directory (default: ./sublane)
  --port PORT        Host port on 127.0.0.1 (default: 8080)
  --runtime MODE     Docker Compose or a Linux binary with a systemd user service
  --proxy PROXY      Generate a Caddyfile or Nginx configuration (default: none)
  --domain DOMAIN    Public HTTPS domain; without one, proxy listens on local HTTP
  --trusted-proxies CIDRS  Proxy source CIDRs as seen by SubLane, comma-separated
  --wait-timeout SECONDS  Startup readiness timeout (default: 90)
  --demo             Install a read-only demo with disposable sample data
  --non-interactive  Use defaults for omitted choices, even with a terminal
  --help             Show this help

An interactive terminal guides the choices. Without one, defaults are a normal
instance, Docker and no proxy; pass flags to choose otherwise. Proxy files are
generated for review but are never installed into Caddy or Nginx automatically.

Requires Bash, curl, and sha256sum or shasum. Docker mode needs Compose;
binary mode needs Linux and a working systemd user manager.
Automatic version selection also requires jq.
Existing directories are never overwritten. Use the deployment guide for upgrades.
Demo mode requires a compatible release and uses the public login demo / sublane-demo.
HELP
}

cleanup() {
  if [[ -n ${install_stage:-} ]]; then rm -rf -- "$install_stage"; fi
}

compose() {
  local directory=$1
  shift
  # Environment interpolation must not select another image, port or existing project.
  SUBLANE_IMAGE="$install_image" SUBLANE_BIND_ADDRESS=127.0.0.1 \
    SUBLANE_PORT="$install_port" SUBLANE_PUBLIC_URL="$install_public_url" \
    SUBLANE_TRUSTED_PROXIES="$install_trusted_proxies" SUBLANE_LOG_LEVEL=info \
    SUBLANE_DEMO="$install_demo" \
    docker compose --project-name "$install_project" --project-directory "$directory" \
    --env-file "$directory/.env" -f "$directory/docker.compose.yaml" "$@" </dev/null
}

prompt_available() {
  [[ ${install_no_prompt:-false} == false ]] && ( : </dev/tty ) 2>/dev/null
}

prompt_choice() {
  local answer
  printf '%s' "$1" >/dev/tty
  IFS= read -r answer </dev/tty || fail 'Could not read the terminal choice.'
  printf '%s' "$answer"
}

validate_domain() {
  [[ ${#install_domain} -le 253 && $install_domain == *.* \
    && $install_domain != .* && $install_domain != *. && $install_domain != *..* ]] \
    || fail 'Use a valid DNS domain for --domain.'
  local label
  local -a labels
  IFS=. read -r -a labels <<< "$install_domain"
  for label in "${labels[@]}"; do
    [[ ${#label} -le 63 && $label =~ ^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$ ]] || fail 'Use a valid DNS domain for --domain.'
  done
}

verify_download() {
  local name=$1 expected actual
  expected=$(awk -v file="$name" '$2 == file { print $1 }' "$install_stage/SHA256SUMS")
  [[ $expected =~ ^[0-9a-f]{64}$ ]] || fail "Release checksums must contain exactly one $name entry."
  actual=$("${install_hash[@]}" "$install_stage/$name")
  [[ ${actual%% *} == "$expected" ]] || fail "$name checksum mismatch; installation stopped."
}

require_demo_support() {
  [[ $install_demo == true ]] || return 0
  # Verified release configuration declares this capability; old releases would silently ignore the flag.
  if ! awk '/^[[:space:]]*SUBLANE_DEMO[:=]/ { supported=1 } END { exit !supported }' "$1" 2>/dev/null; then
    fail "v$install_version does not support demo mode. Choose a compatible release with --version."
  fi
}

download() {
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 10 --max-time 120 --retry 2 \
    --output "$install_stage/$1" "$install_release/$1" || fail "Could not download $1 for v$install_version."
}

validate_version() {
  local version_pattern='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+\.)*[0-9A-Za-z-]+)?$'
  [[ ${#install_version} -lt 100 && $install_version =~ $version_pattern ]] || fail 'Use a canonical version such as 1.2.3 or 1.2.3-rc.1.'
  if [[ $install_version == *-* ]]; then
    local part
    local -a prerelease_parts
    IFS=. read -r -a prerelease_parts <<< "${install_version#*-}"
    for part in "${prerelease_parts[@]}"; do
      [[ ! $part =~ ^0[0-9]+$ ]] || fail 'Numeric prerelease identifiers cannot have leading zeros.'
    done
  fi
}

release_metadata() {
  install_api_status=$(curl --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 10 --max-time 120 --retry 2 --header 'Accept: application/vnd.github+json' \
    --output "$install_stage/releases.json" --write-out '%{http_code}' \
    "https://api.github.com/repos/murongg/SubLane/$1") || fail 'Could not fetch GitHub releases. Retry or pass --version explicitly.'
}

resolve_version() {
  release_metadata releases/latest
  if [[ $install_api_status == 200 ]]; then
    install_version=$(jq -er 'select(.draft == false and .prerelease == false) | .tag_name | select(type == "string")' \
      "$install_stage/releases.json") || fail 'Invalid latest release metadata.'
    install_version=${install_version#v}
    return
  fi
  # Only absence of a stable release permits fallback; rate limits and outages must not change channels.
  [[ $install_api_status == 404 ]] || fail "GitHub latest release request failed (HTTP $install_api_status). Retry or pass --version explicitly."

  local page count candidate published tag newest_date='' newest_tag=''
  for ((page=1; page<=20; page++)); do
    release_metadata "releases?per_page=100&page=$page"
    [[ $install_api_status == 200 ]] || fail "GitHub releases request failed (HTTP $install_api_status)."
    jq -e 'type == "array"' "$install_stage/releases.json" >/dev/null || fail 'Invalid release list.'
    count=$(jq 'length' "$install_stage/releases.json")
    candidate=$(jq -r '[.[] | select(.draft == false and .prerelease == true and (.tag_name | type) == "string" and (.published_at | type) == "string")]
      | max_by(.published_at) | if . == null then empty else [.published_at, .tag_name] | @tsv end' \
      "$install_stage/releases.json")
    if [[ -n $candidate ]]; then
      IFS=$'\t' read -r published tag <<< "$candidate"
      if [[ $published > $newest_date ]]; then
        newest_date=$published
        newest_tag=$tag
      fi
    fi
    if [[ $count -lt 100 ]]; then
      [[ -n $newest_tag ]] || fail 'No published stable or prerelease version is available.'
      install_version=${newest_tag#v}
      return
    fi
  done
  fail 'Release scan limit reached; pass --version explicitly.'
}

generate_proxy_config() {
  case "$install_proxy" in
    none) return ;;
    caddy)
      if [[ -z $install_domain ]]; then
        cat > "$install_stage/Caddyfile" <<CONFIG
http://127.0.0.1 {
    bind 127.0.0.1
    reverse_proxy 127.0.0.1:$install_port
}
CONFIG
      else
        cat > "$install_stage/Caddyfile" <<CONFIG
$install_domain {
    reverse_proxy 127.0.0.1:$install_port
}
CONFIG
      fi
      ;;
    nginx)
      cat > "$install_stage/nginx.conf" <<CONFIG
# Place map inside the Nginx http context. Replace certificate paths if needed.
map \$http_upgrade \$sublane_connection {
    default upgrade;
    ''      close;
}
CONFIG
      local nginx_listen nginx_name
      if [[ -z $install_domain ]]; then
        nginx_listen='127.0.0.1:80'
        nginx_name='_'
      else
        nginx_listen='443 ssl'
        nginx_name=$install_domain
        cat >> "$install_stage/nginx.conf" <<CONFIG

server {
    listen 80;
    server_name $install_domain;
    return 301 https://$install_domain\$request_uri;
}
CONFIG
      fi
      cat >> "$install_stage/nginx.conf" <<CONFIG

server {
    listen $nginx_listen;
    server_name $nginx_name;
CONFIG
      if [[ -n $install_domain ]]; then
        cat >> "$install_stage/nginx.conf" <<CONFIG
    ssl_certificate     /etc/letsencrypt/live/$install_domain/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$install_domain/privkey.pem;
CONFIG
      fi
      cat >> "$install_stage/nginx.conf" <<CONFIG

    location / {
        proxy_pass http://127.0.0.1:$install_port;
        proxy_http_version 1.1;
        proxy_set_header Host \$http_host;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \$sublane_connection;
        proxy_buffering off;
        proxy_read_timeout 650s;
        proxy_send_timeout 650s;
        client_max_body_size 128m;
    }

    location /api/settings/backup/ {
        proxy_pass http://127.0.0.1:$install_port;
        proxy_http_version 1.1;
        proxy_set_header Host \$http_host;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_request_buffering off;
        proxy_buffering off;
        proxy_read_timeout 950s;
        proxy_send_timeout 950s;
        client_max_body_size 256m;
    }
}
CONFIG
      ;;
  esac
}

write_docker_env() {
  cat > "$install_stage/.env" <<ENV
COMPOSE_PROJECT_NAME=$install_project
SUBLANE_IMAGE=$install_image
SUBLANE_BIND_ADDRESS=127.0.0.1
SUBLANE_PORT=$install_port
SUBLANE_PUBLIC_URL=$install_public_url
SUBLANE_TRUSTED_PROXIES=$install_trusted_proxies
SUBLANE_LOG_LEVEL=info
SUBLANE_DEMO=$install_demo
SUBLANE_MAX_REQUEST_BODY_MB=128
ENV
}

write_binary_files() {
  {
    printf 'SUBLANE_ADDR=%q\n' "127.0.0.1:$install_port"
    printf 'SUBLANE_DATA_DIR=%q\n' "$install_dir/data"
    printf 'SUBLANE_PUBLIC_URL=%q\n' "$install_public_url"
    printf 'SUBLANE_TRUSTED_PROXIES=%q\n' "$install_trusted_proxies"
    printf 'SUBLANE_LOG_LEVEL=info\n'
    printf 'SUBLANE_DEMO=%s\n' "$install_demo"
    printf 'SUBLANE_MAX_REQUEST_BODY_MB=128\n'
  } > "$install_stage/sublane.env"
  cat > "$install_stage/start.sh" <<'RUN'
#!/usr/bin/env bash
set -euo pipefail
install_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
set -a
. "$install_root/sublane.env"
set +a
exec "$install_root/sublane"
RUN
  chmod 0700 "$install_stage/start.sh"
  cat > "$install_stage/$install_unit" <<UNIT
[Unit]
Description=SubLane instance $install_project
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/bin/bash "$install_dir/start.sh"
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
UNIT
}

show_configuration() {
  printf '\nInstallation plan: %s v%s in %s\n' "$install_runtime" "$install_version" "$install_dir"
  printf 'Proxy: %s; local port: %s\n' "$install_proxy" "$install_port"
  if [[ $install_demo == true ]]; then
    printf 'Mode: read-only demo. Sample data resets on restart; live API calls are disabled.\n'
  else
    printf 'Mode: normal instance.\n'
  fi
  if [[ $install_runtime == docker ]]; then cat "$install_stage/.env"
  else cat "$install_stage/sublane.env" "$install_stage/$install_unit"
  fi
  case "$install_proxy" in
    caddy) cat "$install_stage/Caddyfile" ;;
    nginx) cat "$install_stage/nginx.conf" ;;
  esac
  if [[ $install_runtime == docker && $install_proxy != none && -z $install_trusted_proxies ]]; then
    printf 'Note: Docker may hide the proxy source IP. Set SUBLANE_TRUSTED_PROXIES after verifying the peer address seen by SubLane.\n'
  fi
  if [[ $install_prompted == true ]]; then
    local answer
    answer=$(prompt_choice 'Proceed with this installation? [y/N]: ')
    [[ $answer == y || $answer == Y ]] || fail 'Installation canceled before deployment.'
  fi
}

install_docker() {
  compose "$install_stage" config --quiet
  printf 'Pulling %s...\n' "$install_image"
  compose "$install_stage" pull || fail 'Image pull failed; no installation was created.'
  mkdir -p -- "$(dirname "$install_dir")"
  # Claim the directory atomically only after verification; simultaneous installers must not overwrite it.
  mkdir -- "$install_dir" || fail "Could not create $install_dir; existing files were left unchanged."
  cp "$install_stage/docker.compose.yaml" "$install_stage/.env" "$install_dir/"
  case "$install_proxy" in
    caddy) cp "$install_stage/Caddyfile" "$install_dir/" ;;
    nginx) cp "$install_stage/nginx.conf" "$install_dir/" ;;
  esac
  if ! compose "$install_dir" up --detach --wait --wait-timeout "$install_wait_timeout" --no-build; then
    printf 'Configuration and any container data are retained in the deployment at %s.\n' "$install_dir" >&2
    printf 'Inspect it with: cd %q && docker compose -f docker.compose.yaml logs --tail=100 sublane\n' "$install_dir" >&2
    fail 'SubLane did not become healthy.'
  fi
  printf 'Manage this deployment: cd %q && docker compose -f docker.compose.yaml ps\n' "$install_dir"
}

install_binary() {
  mkdir -p -- "$(dirname "$install_dir")"
  mkdir -- "$install_dir" || fail "Could not create $install_dir; existing files were left unchanged."
  cp -R "$install_stage/unpacked/." "$install_dir/"
  cp "$install_stage/sublane.env" "$install_stage/start.sh" "$install_stage/$install_unit" "$install_dir/"
  case "$install_proxy" in
    caddy) cp "$install_stage/Caddyfile" "$install_dir/" ;;
    nginx) cp "$install_stage/nginx.conf" "$install_dir/" ;;
  esac
  mkdir -m 0700 "$install_dir/data"
  if ! systemctl --user link "$install_dir/$install_unit" \
    || ! systemctl --user daemon-reload \
    || ! systemctl --user enable --now "$install_unit"; then
    systemctl --user disable --now "$install_unit" >/dev/null 2>&1 || true
    printf 'Verified binary and configuration are retained at %s.\n' "$install_dir" >&2
    fail 'Could not enable the systemd user service. Review the unit and user manager.'
  fi
  local attempt ready=false
  for ((attempt=0; attempt<install_wait_timeout; attempt++)); do
    if systemctl --user is-active --quiet "$install_unit" \
      && curl --fail --silent --max-time 2 "http://127.0.0.1:$install_port/readyz" >/dev/null 2>&1; then
      ready=true
      break
    fi
    if ((attempt+1 < install_wait_timeout)); then sleep 1; fi
  done
  if [[ $ready != true ]]; then
    systemctl --user disable --now "$install_unit" >/dev/null 2>&1 || true
    printf 'Verified binary and configuration are retained at %s.\n' "$install_dir" >&2
    fail "The systemd service did not become ready within $install_wait_timeout seconds."
  fi
  printf 'Manage this deployment: systemctl --user status %s\n' "$install_unit"
  printf 'For service availability after logout, check: loginctl show-user %q -p Linger\n' "${USER:-$(id -un)}"
}

main() {
  install_version=''
  install_dir=$PWD/sublane
  install_port=8080
  install_wait_timeout=90
  install_runtime=''
  install_demo=''
  install_proxy=''
  install_domain=''
  install_trusted_proxies=''
  install_no_prompt=false
  install_prompted=false
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version|--dir|--port|--runtime|--proxy|--domain|--trusted-proxies|--wait-timeout)
        [[ $# -ge 2 && -n $2 ]] || fail "Missing value for $1."
        case "$1" in
          --version) install_version=${2#v}; validate_version ;;
          --dir) install_dir=$2 ;;
          --port) install_port=$2 ;;
          --runtime) install_runtime=$2 ;;
          --proxy) install_proxy=$2 ;;
          --domain) install_domain=$2 ;;
          --trusted-proxies) install_trusted_proxies=$2 ;;
          --wait-timeout) install_wait_timeout=$2 ;;
        esac
        shift 2
        ;;
      --non-interactive) install_no_prompt=true; shift ;;
      --demo) install_demo=true; shift ;;
      --help|-h) usage; return ;;
      *) fail "Unknown argument: $1. Use --help." ;;
    esac
  done

  if [[ -z $install_runtime ]] && prompt_available; then
    local choice
    choice=$(prompt_choice 'Runtime [1 Docker, 2 Linux binary] (default 1): ')
    case "$choice" in
      ''|1) install_runtime=docker ;;
      2) install_runtime=binary ;;
      *) fail 'Choose Docker (1) or Linux binary (2).' ;;
    esac
    install_prompted=true
  fi
  install_runtime=${install_runtime:-docker}
  [[ $install_runtime == docker || $install_runtime == binary ]] || fail 'Use --runtime docker or binary.'

  if [[ -z $install_demo ]] && prompt_available; then
    local choice
    choice=$(prompt_choice 'Instance mode [1 normal, 2 read-only demo] (default 1): ')
    case "$choice" in
      ''|1) install_demo=false ;;
      2) install_demo=true ;;
      *) fail 'Choose a normal instance (1) or read-only demo (2).' ;;
    esac
    install_prompted=true
  fi
  install_demo=${install_demo:-false}

  if [[ -z $install_proxy ]] && prompt_available; then
    local choice
    choice=$(prompt_choice 'Reverse proxy [1 none, 2 Caddy, 3 Nginx] (default 1): ')
    case "$choice" in
      ''|1) install_proxy=none ;;
      2) install_proxy=caddy ;;
      3) install_proxy=nginx ;;
      *) fail 'Choose no proxy (1), Caddy (2), or Nginx (3).' ;;
    esac
    install_prompted=true
  fi
  install_proxy=${install_proxy:-none}
  [[ $install_proxy == none || $install_proxy == caddy || $install_proxy == nginx ]] || fail 'Use --proxy none, caddy, or nginx.'
  if [[ $install_proxy != none && -z $install_domain ]] && prompt_available; then
    printf 'Public HTTPS domain (blank for local HTTP only): ' >/dev/tty
    IFS= read -r install_domain </dev/tty || fail 'Could not read the public domain.'
    install_prompted=true
  fi
  if [[ $install_proxy == none ]]; then
    [[ -z $install_domain && -z $install_trusted_proxies ]] || fail '--domain and --trusted-proxies require a reverse proxy.'
  elif [[ -n $install_domain ]]; then
    validate_domain
  fi
  if [[ -n $install_trusted_proxies ]]; then
    local cidr
    local -a cidrs
    IFS=, read -r -a cidrs <<< "$install_trusted_proxies"
    for cidr in "${cidrs[@]}"; do
      [[ $cidr =~ ^[0-9A-Fa-f:.]+/[0-9]{1,3}$ ]] || fail 'Use comma-separated IP CIDRs for --trusted-proxies.'
    done
  elif [[ $install_runtime == binary && $install_proxy != none ]]; then
    install_trusted_proxies=127.0.0.1/32
  fi
  install_public_url=''
  if [[ -n $install_domain ]]; then install_public_url="https://$install_domain"; fi

  [[ $install_port =~ ^[1-9][0-9]{0,4}$ ]] || fail 'Port must be an integer from 1 to 65535.'
  [[ $install_port -le 65535 ]] || fail 'Port must be an integer from 1 to 65535.'
  [[ $install_wait_timeout =~ ^[1-9][0-9]{0,2}$ && $install_wait_timeout -le 300 ]] \
    || fail 'Startup wait timeout must be 1 to 300 seconds.'
  [[ $install_dir != *$'\n'* && $install_dir != *$'\r'* ]] || fail 'Directory cannot contain line breaks.'
  [[ $install_dir == /* ]] || install_dir=$PWD/$install_dir
  if [[ $install_runtime == binary ]]; then
    case "$install_dir" in
      *'%'*|*'$'*|*'"'*|*$'\\'*) fail 'Binary installation directory cannot contain %, $, quotes, or backslashes.' ;;
    esac
  fi
  [[ ! -e $install_dir && ! -L $install_dir ]] || fail "Directory already exists: $install_dir. Configuration and data were left unchanged."

  local dependency
  for dependency in curl awk mktemp; do
    command -v "$dependency" >/dev/null || fail "Install $dependency first."
  done
  if [[ -z $install_version ]]; then
    command -v jq >/dev/null || fail 'Install jq for automatic version selection, or pass --version explicitly.'
  fi
  if command -v sha256sum >/dev/null; then
    install_hash=(sha256sum)
  elif command -v shasum >/dev/null; then
    install_hash=(shasum -a 256)
  else
    fail 'Install sha256sum or shasum first.'
  fi
  if [[ $install_runtime == docker ]]; then
    command -v docker >/dev/null || fail 'Install Docker first.'
    docker compose version >/dev/null || fail 'Docker Compose is required.'
    docker info >/dev/null 2>&1 || fail 'Docker is not running or the current user cannot access it.'
  else
    for dependency in tar uname systemctl; do
      command -v "$dependency" >/dev/null || fail "Install $dependency first."
    done
    [[ $(uname -s) == Linux ]] || fail 'Published binary installations require Linux.'
    case "$(uname -m)" in
      x86_64) install_arch=amd64 ;;
      aarch64|arm64) install_arch=arm64 ;;
      *) fail 'Published binaries support Linux amd64 and arm64 only.' ;;
    esac
    systemctl --user show-environment >/dev/null 2>&1 || fail 'A running systemd user manager is required for binary mode.'
  fi

  umask 077
  install_stage=$(mktemp -d "${TMPDIR:-/tmp}/sublane-install.XXXXXXXX")
  trap cleanup EXIT
  trap 'exit 130' INT TERM
  if [[ -z $install_version ]]; then resolve_version; fi
  validate_version
  install_release="https://github.com/murongg/SubLane/releases/download/v$install_version"
  install_image="ghcr.io/murongg/sublane:$install_version"
  # Persist a distinct project name so a same-named directory cannot reuse another instance's volume.
  install_project="sublane-$RANDOM-$RANDOM-$$"

  printf 'Downloading deployment files for v%s...\n' "$install_version"
  if [[ $install_runtime == docker ]]; then
    download docker.compose.yaml
    install_artifact=docker.compose.yaml
  else
    install_artifact="sublane_${install_version}_linux_${install_arch}.tar.gz"
    download "$install_artifact"
  fi
  download SHA256SUMS
  verify_download "$install_artifact"
  if [[ $install_runtime == docker ]]; then
    require_demo_support "$install_stage/docker.compose.yaml"
    write_docker_env
  else
    mkdir "$install_stage/unpacked"
    tar -tzf "$install_stage/$install_artifact" > "$install_stage/archive.list" || fail 'Invalid binary archive.'
    local member
    while IFS= read -r member; do
      case "$member" in
        /*|..|../*|*/../*|*/..) fail 'Binary archive contains an unsafe path.' ;;
      esac
    done < "$install_stage/archive.list"
    tar -xzf "$install_stage/$install_artifact" -C "$install_stage/unpacked" || fail 'Could not extract binary archive.'
    [[ -f $install_stage/unpacked/sublane && ! -L $install_stage/unpacked/sublane && -x $install_stage/unpacked/sublane ]] \
      || fail 'Binary archive is missing the executable.'
    require_demo_support "$install_stage/unpacked/.env.example"
    install_unit="$install_project.service"
    write_binary_files
  fi
  generate_proxy_config
  show_configuration
  if [[ $install_runtime == docker ]]; then install_docker; else install_binary; fi
  printf '\nInstallation complete: %s\n' "$install_dir"
  if [[ $install_proxy == none ]]; then
    if [[ $install_demo == true ]]; then
      printf 'Open http://127.0.0.1:%s and choose Explore demo.\n' "$install_port"
    else
      printf 'Open http://127.0.0.1:%s to create the administrator.\n' "$install_port"
    fi
  else
    local proxy_file=${install_proxy/caddy/Caddyfile}
    [[ $install_proxy != nginx ]] || proxy_file=nginx.conf
    if [[ -n $install_domain ]]; then
      printf 'Review %s, install it in %s, then open %s.\n' "$proxy_file" "$install_proxy" "$install_public_url"
    else
      printf 'Review %s, install it in %s, then open http://127.0.0.1.\n' "$proxy_file" "$install_proxy"
    fi
    if [[ $install_runtime == docker && -z $install_trusted_proxies ]]; then
      printf 'Set SUBLANE_TRUSTED_PROXIES to the proxy peer address seen inside the container before public access.\n'
    fi
  fi
  if [[ $install_demo == true ]]; then
    printf 'Demo login: demo / sublane-demo. This instance is read-only and resets sample data on restart.\n'
  fi
}

# Invoke only after the complete function definition has arrived when used through curl | bash.
main "$@"
