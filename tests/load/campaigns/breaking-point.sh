# Breaking-point campaign, several hours: pushes every suite until errors,
# lost data, or missing events. Uploads slower than 5 minutes count as errors.
# Small sizes run first so the main results arrive early; 100M and 1G are
# capped by load-generator memory and disk, so they usually end as lower bounds.
campaign() {
  local common=(UPLOAD_TIMEOUT=5m MAX_DURATION=2h)

  upload_capacity upload-capacity-1K FILE_SIZE=1K MAX_VUS=1024 UPLOAD_TARGET=personal "${common[@]}"
  upload_capacity upload-capacity-100K FILE_SIZE=100K MAX_VUS=1024 UPLOAD_TARGET=personal "${common[@]}"
  upload_capacity upload-capacity-1M FILE_SIZE=1M MAX_VUS=1024 UPLOAD_TARGET=personal "${common[@]}"

  if shared_drive_prepare; then
    upload_capacity shared-drive-upload-capacity-1M FILE_SIZE=1M MAX_VUS=1024 UPLOAD_TARGET=shared-drive "${common[@]}"
    shared_drive_ramp shared-drive-ramp RAMP_LEVELS=1,2,4,8,16,32,64
  fi

  upload_capacity upload-capacity-10M FILE_SIZE=10M MAX_VUS=128 UPLOAD_TARGET=personal "${common[@]}"
  upload_capacity upload-capacity-100M FILE_SIZE=100M UPLOAD_TARGET=personal "${common[@]}"
  upload_capacity upload-capacity-1G FILE_SIZE=1G UPLOAD_TARGET=personal BASELINE_ITERATIONS=10 CONFIRMATION_ITERATIONS=10 "${common[@]}"
}
