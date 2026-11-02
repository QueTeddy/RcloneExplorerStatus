# Continue a broken upload of data1.doi

`aws s3 cp` cannot continue after a drop. When that command fails, it aborts the multipart upload. The upload ID in the error is already gone, including the one that died on part 2250 with `EOF occurred in violation of protocol`. Running `aws s3 cp` again starts from byte 0.

Use `scripts/resume-s3-upload.ps1` instead. The first run opens one multipart upload and saves its ID. Each part that reaches S3 stays there. If the network dies, run the same command again. The script asks S3 which parts exist and sends only the missing ones.

## Continue

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\resume-s3-upload.ps1
```

Run that from the repo root. Run it again after any break. Exit code `2` means it stopped early and the same command will continue. Exit code `0` means the object is complete and its size matches the local file.

Leave this file where it is until the script prints `Upload complete`:

`C:\Users\Admin\AppData\Local\rclone\vfs\s3\local-sync-folder-reclone\games\Call of Duty Modern Warfare Campaign [DODI Repack]\data1.doi`

The script reads that file in 64 MB pieces. It does not make a second 71 GB copy on `C:`.

## What not to delete

The resume ID is stored here:

`%LOCALAPPDATA%\RcloneExplorerStatus\s3-resume-data1.json`

Keep that file until the upload completes. Deleting it loses the ID, and the next run starts over. The log is `%LOCALAPPDATA%\RcloneExplorerStatus\logs\s3-resume.log`.

Do not run `aws s3 cp` on this key while the script is in progress. That starts a different upload and does not add parts to this one.

## What the script does

1. Creates the multipart upload on `s3://local-sync-folder-reclone/games/Call of Duty Modern Warfare Campaign [DODI Repack]/data1.doi` in `us-east-1`, or loads the saved upload ID.
2. Lists parts already stored.
3. Reads the next missing 64 MB slice from the local file and calls `aws s3api upload-part`.
4. Retries that part five times. If it is still failing, the script exits and leaves the finished parts in place.
5. When every part is present, it calls `complete-multipart-upload` and checks the object size against the local file (76,717,011,562 bytes).

A 64 MB part size is about 1,143 parts. S3 allows 10,000 parts, so this stays under the limit. The part size is fixed for the life of the upload. The script refuses to continue if the local file size changes.

## Start over on purpose

This throws away the in-progress parts for the saved upload only:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\resume-s3-upload.ps1 -Abort
```

The next run without `-Abort` begins a new upload.

## Old incomplete uploads

Earlier attempts are still open on this key and are not used by the script. After `Upload complete`, list them and abort each `UploadId`:

```powershell
aws s3api list-multipart-uploads --bucket local-sync-folder-reclone --region us-east-1 --prefix "games/Call of Duty Modern Warfare Campaign [DODI Repack]/data1.doi"
```

```powershell
aws s3api abort-multipart-upload --bucket local-sync-folder-reclone --region us-east-1 --key "games/Call of Duty Modern Warfare Campaign [DODI Repack]/data1.doi" --upload-id "UPLOAD_ID"
```

Those leftovers are from the rclone attempt and from `aws s3 cp`. None of them is the part-2250 upload. That one was aborted when `aws s3 cp` failed.
