#!/bin/bash
set -e

rundir="${1}"

usage() {
  >&2 echo -e "Usage: $0 [dir]"
}

if [ -z "${rundir}" ]; then
  usage
  exit 1
fi

if [ -f "${rundir}" ]; then
  runfile="/usr/src/konnector/$(basename "${rundir}")"
  rundir="$(dirname "${rundir}")"
elif [ -d "${rundir}" ]; then
  runfile="/usr/src/konnector"
else
  >&2 echo "${rundir} does not exist"
  exit 1
fi

if [ -z "${COZY_JOB_ID}" ]; then
  COZY_JOB_ID="unknown"
fi

log_name=$(echo "${COZY_JOB_ID}" | tr A-Z a-z | sed -e 's/[^a-z0-9\-]/-/g')

read -r -d '' seccomp_string << EOM
// This seccomp policy is inspired by the following resources:
//
//   https://docs.docker.com/engine/security/seccomp/#significant-syscalls-blocked-by-the-default-profile
//   https://github.com/moby/moby/blob/4f259698b07653e9e5220e097df79862f9e54b74/profiles/seccomp/seccomp_default.go
//   https://github.com/sandstorm-io/sandstorm/blob/dbc66bd315e87910dab868bc85352c3880e9d716/src/sandstorm/supervisor.c%2B%2B#L1069-L1220
//
// Only allow AF_INET and AF_INET6 protocols with SOCK_STREAM and SOCK_DGRAM
// types of socket for TCP and UDP families.

/* Supported address families. */
#define AF_INET   2   /* Internet IP Protocol   */
#define AF_INET6  10  /* IP version 6 */

#define SOCK_STREAM 1   /* stream socket */
#define SOCK_DGRAM  2   /* datagram socket */
#define SOCK_TYPE_MASK 0x0f

POLICY konnectors {
  KILL {
    acct,
    add_key,
    adjtimex,
    bpf,
    clock_adjtime,
    clock_settime,
    create_module,
    delete_module,
    finit_module,
    get_kernel_syms,
    get_mempolicy,
    init_module,
    io_cancel,
    io_destroy,
    io_getevents,
    io_setup,
    io_submit,
    ioperm,
    iopl,
    kcmp,
    keyctl,
    kexec_file_load,
    kexec_load,
    lookup_dcookie,
    mbind,
    migrate_pages,
    modify_ldt,
    mount,
    move_pages,
    name_to_handle_at,
    nfsservctl,
    open_by_handle_at,
    perf_event_open,
    personality,
    pivot_root,
    query_module,
    process_vm_readv,
    process_vm_writev,
    ptrace,
    quotactl,
    reboot,
    remap_file_pages,
    request_key,
    seccomp,
    set_mempolicy,
    set_thread_area,
    setns,
    settimeofday,
    syslog,
    swapon,
    swapoff,
    sysfs,
    umount,
    unshare,
    uselib,
    userfaultfd,
    vmsplice
  },
  ERRNO(57) { /* EAFNOSUPPORT = address family not supported */
    socket(domain, type) {
      (domain != AF_INET && domain != AF_INET6) ||
      ((type & SOCK_TYPE_MASK) != SOCK_STREAM &&
       (type & SOCK_TYPE_MASK) != SOCK_DGRAM)
    }
  }
}
USE konnectors DEFAULT ALLOW
EOM

nsjail \
  --quiet \
  --mode o \
  --rlimit_as 2048 \
  --rlimit_cpu 1000 \
  --rlimit_fsize 1024 \
  --rlimit_nofile 128 \
  --rlimit_nproc 512 \
  --time_limit "${COZY_TIME_LIMIT}" \
  --disable_proc \
  --disable_clone_newnet \
  --iface_no_lo \
  --seccomp_string "${seccomp_string}" \
  --log "nsjail-${log_name}.log" \
  -E "COZY_URL=${COZY_URL}" \
  -E "COZY_FIELDS=${COZY_FIELDS}" \
  -E "COZY_PARAMETERS=${COZY_PARAMETERS}" \
  -E "COZY_CREDENTIALS=${COZY_CREDENTIALS}" \
  -E "COZY_LOCALE=${COZY_LOCALE}" \
  -E "COZY_JOB_MANUAL_EXECUTION=${COZY_JOB_MANUAL_EXECUTION}" \
  -R "${rundir}:/usr/src/konnector/" \
  -R /lib \
  -R /lib64 \
  -R /usr/lib \
  -R /usr/bin/nodejs \
  -R /dev/urandom \
  -R /etc/resolv.conf \
  -R /etc/ssl/certs \
  -- /usr/bin/nodejs "${runfile}"

# Via a chroot with nodejs installed inside
# nsjail \
#   --quiet \
#   --chroot /chrootdir \
#   --rlimit_as 2048 \
#   --rlimit_cpu 1000 \
#   --rlimit_fsize 1024 \
#   --rlimit_nofile 128 \
#   --rlimit_nproc 512 \
#   --time_limit "${COZY_TIME_LIMIT}" \
#   --disable_proc \
#   --disable_clone_newnet \
#   --iface_no_lo \
#   --seccomp_string "${seccomp_string}" \
#   -E "COZY_URL=${COZY_URL}" \
#   -E "COZY_FIELDS=${COZY_FIELDS}" \
#   -E "COZY_PARAMETERS=${COZY_PARAMETERS}" \
#   -E "COZY_CREDENTIALS=${COZY_CREDENTIALS}" \
#   -E "COZY_JOB_MANUAL_EXECUTION=${COZY_JOB_MANUAL_EXECUTION}" \
#   -E "COZY_LOCALE=${COZY_LOCALE}" \
#   -R "${rundir}:/usr/src/konnector/" \
#   -- /usr/bin/nodejs "${runfile}"
