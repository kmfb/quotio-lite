#!/bin/zsh
set -euo pipefail

SCRIPT_DIR=${0:A:h}
REPO_ROOT=${SCRIPT_DIR:h}
SCRIPT_NAME=${0:t}
SERVICE_LABEL=${QUOTIO_LITE_SERVICE_LABEL:-io.github.kmfb.quotio-lite}
SERVICE_ROOT=${QUOTIO_LITE_SERVICE_ROOT:-${HOME}/.quotio-lite/service}
INSTALL_BIN=${SERVICE_ROOT}/bin/quotio-lite
INSTALL_WEB=${SERVICE_ROOT}/web
LOG_DIR=${SERVICE_ROOT}/logs
STDOUT_LOG=${LOG_DIR}/quotio-lite.stdout.log
STDERR_LOG=${LOG_DIR}/quotio-lite.stderr.log
PLIST_PATH=${QUOTIO_LITE_SERVICE_PLIST:-${HOME}/Library/LaunchAgents/${SERVICE_LABEL}.plist}
BUILD_BIN=${REPO_ROOT}/build/quotio-lite
BUILD_WEB=${REPO_ROOT}/web/dist
TEMPLATE_PATH=${REPO_ROOT}/service/io.github.kmfb.quotio-lite.plist.template
LAUNCHD_DOMAIN=gui/$(id -u)
JOB_TARGET=${LAUNCHD_DOMAIN}/${SERVICE_LABEL}
SERVICE_PORT=${QUOTIO_LITE_PORT:-18417}

usage() {
  cat <<USAGE
Usage: ${SCRIPT_NAME} <install|start|stop|restart|status|logs|uninstall|render-plist>
USAGE
}

require_file() {
  local path=$1
  local hint=$2
  if [[ ! -e ${path} ]]; then
    echo "Missing ${path}. ${hint}" >&2
    exit 1
  fi
}

job_loaded() {
  launchctl print "${JOB_TARGET}" >/dev/null 2>&1
}

port_listener() {
  lsof -nP -iTCP:${SERVICE_PORT} -sTCP:LISTEN 2>/dev/null || true
}

port_in_use_by_other() {
  local output
  output=$(port_listener)
  [[ -n ${output} ]] || return 1

  if [[ ${output} == *${INSTALL_BIN}* ]]; then
    return 1
  fi
  return 0
}

render_plist() {
  require_file "${TEMPLATE_PATH}" "The launchd plist template is missing."
  LABEL="${SERVICE_LABEL}" \
  INSTALL_BIN="${INSTALL_BIN}" \
  SERVICE_ROOT="${SERVICE_ROOT}" \
  INSTALL_WEB="${INSTALL_WEB}" \
  STDOUT_LOG="${STDOUT_LOG}" \
  STDERR_LOG="${STDERR_LOG}" \
  perl -0pe 's#__LABEL__#$ENV{LABEL}#g; s#__INSTALL_BIN__#$ENV{INSTALL_BIN}#g; s#__SERVICE_ROOT__#$ENV{SERVICE_ROOT}#g; s#__INSTALL_WEB__#$ENV{INSTALL_WEB}#g; s#__STDOUT_LOG__#$ENV{STDOUT_LOG}#g; s#__STDERR_LOG__#$ENV{STDERR_LOG}#g' "${TEMPLATE_PATH}"
}

install_service() {
  require_file "${BUILD_BIN}" "Run `make build` first."
  require_file "${BUILD_WEB}/index.html" "Run `make build` first so the frontend bundle exists."

  mkdir -p "${SERVICE_ROOT}/bin" "${INSTALL_WEB}" "${LOG_DIR}" "${HOME}/Library/LaunchAgents"
  rm -rf "${INSTALL_WEB}"
  mkdir -p "${INSTALL_WEB}"

  cp "${BUILD_BIN}" "${INSTALL_BIN}"
  chmod +x "${INSTALL_BIN}"
  cp -R "${BUILD_WEB}/." "${INSTALL_WEB}/"
  render_plist > "${PLIST_PATH}"

  echo "Installed launchd bundle: ${PLIST_PATH}"
  echo "Binary: ${INSTALL_BIN}"
  echo "Frontend: ${INSTALL_WEB}"
  if job_loaded; then
    echo "Launchd job is currently loaded; run '${SCRIPT_NAME} restart' when ready to pick up the new bundle."
  else
    echo "Launchd job is not loaded yet; run '${SCRIPT_NAME} start' when ready."
  fi
}

start_service() {
  require_file "${PLIST_PATH}" "Run `make service-install` first."
  if ! job_loaded && port_in_use_by_other; then
    echo "Port ${SERVICE_PORT} is already in use. Refusing to start the launchd service so an existing dev instance is not disrupted." >&2
    port_listener >&2
    exit 1
  fi

  if job_loaded; then
    launchctl kickstart -k "${JOB_TARGET}"
  else
    launchctl bootstrap "${LAUNCHD_DOMAIN}" "${PLIST_PATH}"
    launchctl kickstart -k "${JOB_TARGET}"
  fi

  status_service
}

stop_service() {
  if job_loaded; then
    launchctl bootout "${JOB_TARGET}" >/dev/null 2>&1 || launchctl bootout "${LAUNCHD_DOMAIN}" "${PLIST_PATH}"
    echo "Stopped ${SERVICE_LABEL}."
  else
    echo "${SERVICE_LABEL} is not loaded."
  fi
}

status_service() {
  echo "Label: ${SERVICE_LABEL}"
  echo "Plist: ${PLIST_PATH}"
  echo "Binary: ${INSTALL_BIN}"
  echo "Frontend: ${INSTALL_WEB}"
  echo "Stdout log: ${STDOUT_LOG}"
  echo "Stderr log: ${STDERR_LOG}"
  if job_loaded; then
    echo "Launchd: loaded"
    launchctl print "${JOB_TARGET}" | sed -n '1,40p'
  else
    echo "Launchd: not loaded"
  fi

  local listener
  listener=$(port_listener)
  if [[ -n ${listener} ]]; then
    echo "Port ${SERVICE_PORT}:"
    echo "${listener}"
  else
    echo "Port ${SERVICE_PORT}: not listening"
  fi
}

logs_service() {
  mkdir -p "${LOG_DIR}"
  touch "${STDOUT_LOG}" "${STDERR_LOG}"
  tail -n 200 -f "${STDOUT_LOG}" "${STDERR_LOG}"
}

uninstall_service() {
  if job_loaded; then
    stop_service
  fi
  rm -f "${PLIST_PATH}"
  rm -rf "${SERVICE_ROOT}"
  echo "Removed ${SERVICE_LABEL}."
}

case ${1:-} in
  install)
    install_service
    ;;
  start)
    start_service
    ;;
  stop)
    stop_service
    ;;
  restart)
    stop_service
    start_service
    ;;
  status)
    status_service
    ;;
  logs)
    logs_service
    ;;
  uninstall)
    uninstall_service
    ;;
  render-plist)
    render_plist
    ;;
  *)
    usage >&2
    exit 1
    ;;
esac
