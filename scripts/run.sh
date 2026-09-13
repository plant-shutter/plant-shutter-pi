#!/bin/sh
set -eu

base=$(
  CDPATH= cd -- "$(dirname -- "$0")" && pwd
)
service_name=plant-shutter.service

install_service() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "run.sh install must be run as root (try: sudo ./run.sh install)" >&2
    exit 1
  fi
  command -v systemctl >/dev/null || { echo "systemd is required for install" >&2; exit 1; }
  [ -x "$base/bin/plant-shutter" ] || { echo "Missing executable: $base/bin/plant-shutter" >&2; exit 1; }
  [ -f "$base/lib/libobjectbox.so" ] || { echo "Missing library: $base/lib/libobjectbox.so" >&2; exit 1; }

  if [ "$#" -ne 0 ]; then
    echo "run.sh install does not accept application arguments" >&2
    exit 1
  fi
  # Resolve a relative DATA_DIR against the package, just as the service will
  # do after a reboot.
  data_dir=${DATA_DIR:-$base/projects}
  case "$data_dir" in /*) ;; *) data_dir="$base/$data_dir" ;; esac
  mkdir -p "$base/static"

  was_active=false
  if systemctl is-active --quiet "$service_name"; then was_active=true; fi

  unit_path="/etc/systemd/system/$service_name"
  unit_tmp=$(mktemp "$unit_path.XXXXXX")
  trap 'rm -f "$unit_tmp"' EXIT
  {
    cat <<UNIT
[Unit]
Description=Plant Shutter Camera
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
WorkingDirectory=$base
ExecStart=$base/run.sh start
Environment=PORT=${PORT:-8081}
Environment=DATA_DIR=$data_dir
Environment=LD_LIBRARY_PATH=$base/lib
UNIT
    cat <<UNIT
Restart=on-failure
RestartSec=3
StandardOutput=append:$base/plant-shutter.log
StandardError=append:$base/plant-shutter.log

[Install]
WantedBy=multi-user.target
UNIT
  } > "$unit_tmp"
  chmod 644 "$unit_tmp"
  mv "$unit_tmp" "$unit_path"
  trap - EXIT

  systemctl daemon-reload
  systemctl enable --now "$service_name"
  # enable --now does not restart a service that is already running.
  if [ "$was_active" = true ]; then systemctl restart "$service_name"; fi
  echo "Installed and started $service_name"
  echo "Log: $base/plant-shutter.log"
}

case "${1:-}" in
  install)
    shift
    install_service "$@"
    exit 0
    ;;
  start)
    shift
    ;;
esac

cd -- "$base"
exec env LD_LIBRARY_PATH="$base/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}" \
  "$base/bin/plant-shutter" \
  -port "${PORT:-8081}" \
  -statics "$base/static" \
  -dir "${DATA_DIR:-$base/projects}" \
  "$@"
