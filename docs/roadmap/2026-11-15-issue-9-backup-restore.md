# Backup and isolated restore verification

Source: [issue #9](https://github.com/DiiKhanh/worklane/issues/9).

## Status and runtime evidence

**Incomplete, awaiting approval.** No new backup or restore has been performed.
The following observations are from 2026-11-15 17:32-17:33 UTC on VPS/node `dk`,
using `/home/dk/.kube/config`, namespace `worklane`. Repository baseline:
`a3f60f684cd0c2c4d1191aa52d17f8b36d2f6ade`.

- Live `mysql-backup`: schedule `0 18 * * *`, `suspend=true`, concurrency policy
  `Forbid`, lastScheduleTime `2026-08-25T18:00:00Z`, no lastSuccessfulTime.
  No retained backup Jobs exist; only the completed seed Job remains. This does
  not prove that no historical backup ever ran.
- Secret `worklane/r2-backup` is absent. No R2 credentials, bucket access or stored
  object could be verified. The pending runbook is consistent with this gap;
  neither the manifest nor suspension proves a successful backup.
- MySQL is `8.0.46`, running image
  `docker.io/library/mysql@sha256:7dcddc01f13bab2f15cde676d44d01f61fc9f99fe7785e86196dfc07d358ae2b`.
  All nine application tables are InnoDB. OTP migration is 4, identity migration
  is 1, both clean. Expected tables: `identity.api_keys`, `identity.tenants`,
  `identity.users`, `identity.schema_migrations`, `otp.delivery_logs`,
  `otp.otp_requests`, `otp.templates`, `otp.template_versions`,
  `otp.schema_migrations`.
- Database directory: 203,924 KiB. Application table data plus indexes total
  294,912 bytes according to information_schema, an estimate rather than a dump
  size. Root filesystem: 5,489,700,864 bytes available, 70% used.
- Host: 4 CPUs, 5,898 MiB RAM, 3,575 MiB available, no swap. Kubernetes requests
  total 650m CPU/1,712 MiB; limits total 2,800m CPU/3,498 MiB. Node Ready, all
  pressure conditions False. Metrics API unavailable; these are host readings
  and resource declarations, not measured pod utilization.
- All seven long-running workloads were Ready. They restarted about four minutes
  before inspection; this observation does not establish the cause.
- OTP API/dispatcher use source `14f2158340608a5caae9f7b53ffc85f0395c7834`,
  with digests recorded in the [upgrade evidence](2026-11-15-issue-7-backend-upgrade.md).
  Auth uses `sha-041efd7`. No deployed version was changed during inspection.

The earlier private pre-upgrade dump covers only OTP and has no restore proof.
It cannot satisfy this ticket's two-database requirement. No secret values,
application rows, email addresses or OTP values were printed during inspection.

## Proposed operation and approval scope

Prefer a direct local backup and isolated restore over provisioning R2 for this
drill. It uses the existing cached MySQL image and does not require cloud charges,
new external storage, access changes or credentials. Local storage shares the
VPS failure domain, so this proves restoreability, not off-host disaster recovery.
R2 provisioning is a separate decision. Scheduling and stored-object gaps remain
explicit even if a local restore succeeds.

Requested approval covers a read-only transactional dump of both live databases,
private local backup/evidence files, and one isolated temporary MySQL Pod on the
VPS. No live database import, schema change, application deployment, CronJob
change, notification, external upload or cleanup is included.

The procedure below must stop on a failed command or verification mismatch.
Retain failed artifacts for inspection; do not automatically remove anything.

1. Recheck disk, memory, node pressure, MySQL readiness and image identity just
   before execution. Require at least 4 GiB free disk and 2 GiB available RAM.
   Postpone during deployments or schema changes: single-transaction dumps
   protect InnoDB data consistency but not concurrent DDL.
2. Use Bash with `set -euo pipefail` and `umask 077`. Create a unique directory
   under `/home/dk/.local/state/worklane/backups/issue-9-<UTC timestamp>` (0700).
   Set `KUBECONFIG=/home/dk/.kube/config`. Keep SQL, stderr and raw comparison
   files inside that directory (0600); never include their contents in git or chat.
3. Capture the backup with the following command, redirecting stdout to
   `source.sql` and stderr to `source.stderr` in that private directory. Require
   exit status zero before using the output. Record UTC start/end, actual byte
   size, SHA-256, source Pod UID and running imageID.

   ```sh
   kubectl -n worklane exec mysql-0 -- sh -ec '
     export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
     exec mysqldump -uroot --single-transaction --quick --skip-lock-tables \
       --routines --events --triggers --hex-blob --order-by-primary \
       --set-gtid-purged=OFF --skip-comments --skip-dump-date \
       --databases otp identity
   '
   ```

4. Create only Pod `worklane/mysql-restore-issue-9`, using the exact MySQL digest
   above, `restartPolicy: Never`, `automountServiceAccountToken: false`, no
   Service, host ports, host mounts or application labels. Use a disk-backed
   `emptyDir` mounted at `/var/lib/mysql`, sizeLimit 1 GiB. Request 100m CPU,
   512 MiB RAM and 512 MiB ephemeral storage; limit 500m CPU, 1 GiB RAM and
   1536 MiB ephemeral storage. Abort rather than replacing an existing Pod.
   The container command is:

   ```sh
   sh -ec '
     umask 077
     head -c 32 /dev/urandom | base64 > /tmp/.drill-password
     export MYSQL_ROOT_PASSWORD="$(cat /tmp/.drill-password)"
     exec /usr/local/bin/docker-entrypoint.sh mysqld --skip-networking \
       --innodb-buffer-pool-size=128M --event-scheduler=OFF
   '
   ```

   The generated password remains inside the isolated Pod; it is never printed.
   MySQL listens only on its Unix socket. No production credentials or Secret
   objects are mounted in the restore Pod. Allow up to 180 seconds for an
   authenticated `SELECT 1` through `kubectl exec`; stop if readiness fails.
5. Stream `source.sql` to the isolated Pod only:

   ```sh
   kubectl -n worklane exec -i mysql-restore-issue-9 -- sh -ec '
     export MYSQL_PWD="$(cat /tmp/.drill-password)"
     exec mysql -uroot
   ' < "$DRILL_DIR/source.sql" 2> "$DRILL_DIR/restore.stderr"
   ```

6. Query the restored database through its local socket. Verify the nine expected
   tables, InnoDB engines, exact row counts for every table, OTP migration 4 and
   identity migration 1 with dirty=0. Store only names, counts and migration
   versions in the redacted evidence. Empty tables can be valid; do not invent
   data or require every table to have rows.
7. Run the identical mysqldump command against the restore Pod, replacing only
   the credential export with the private password-file read. Save to
   `restored.sql` with private stderr. Require `cmp source.sql restored.sql`
   to succeed and record both SHA-256 values. This compares schema, ordered
   data, triggers, routines and events with the actual source snapshot, rather
   than assuming a later live row count is the same snapshot. If output differs,
   inspect privately, distinguish serialization differences from data loss and
   leave verification incomplete until explained. Do not publish SQL diffs.
8. Record completion timestamp, image identity, backup identity, restored counts,
   comparison result, resource use and post-drill live workload health in this
   document. Run no business requests and send no notifications. Leave the
   isolated Pod and private files intact pending separate cleanup approval.

The scratch instance adds up to 1 GiB RAM, 500m CPU and 1536 MiB ephemeral storage
to the VPS while it is retained. Kubernetes storage limits are eviction limits,
not filesystem quotas; monitor free space during execution and stop on pressure.
Evidence access remains restricted to the existing operator and cluster
administrators. Cleanup approval must identify the exact Pod and private paths;
deleting the Pod deletes the restored data in its emptyDir.

## Acceptance and local validation

- Schedule, retained runs and absent R2 secret: inspected at runtime.
- Latest R2 stored object: unverified because configured access is absent.
- Capacity and proposed backup/restore procedure: recorded for approval.
- New two-database backup identity and successful isolated restore: pending.
- Private artifact preservation and cleanup boundary: specified, not executed.

No application code changed, so no regression test seam or TDD cycle applies.
The relevant end-to-end check is the actual backup/import/snapshot comparison;
it requires the approval above. Static documentation review cannot replace it.

`git diff --check` passed. The requested local checks were attempted:
`make test` could not start because make is absent; dashboard
`pnpm typecheck && pnpm lint && pnpm test` could not start because pnpm is absent.
No passing application-test or typecheck result is claimed for this change.
