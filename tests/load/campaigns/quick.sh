# Quick capacity check, about 30 to 60 minutes: 1 MiB upload capacity on the
# personal drive and on a shared drive, then the shared-drive ramp to level 16.
campaign() {
  upload_capacity upload-capacity-1M FILE_SIZE=1M UPLOAD_TARGET=personal

  if shared_drive_prepare; then
    upload_capacity shared-drive-upload-capacity-1M FILE_SIZE=1M UPLOAD_TARGET=shared-drive
    shared_drive_ramp shared-drive-ramp RAMP_LEVELS=1,2,4,8,16
  fi
}
