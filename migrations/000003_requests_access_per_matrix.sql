-- One-off data fix: 000003_requests_access_per_matrix.sql
-- Description: Brings role permissions in line with the module access matrix of
--              the requirement document ("Matriks akses modul per role"):
--                Requests  -> Admin, Asset Team, Sales Admin only
--                Approvals -> Admin and Sales Supervisor / RSM / GRSM / NSM / Sales Director
--              The seed (database.MigrateAndSeed) only ever adds permissions, so a
--              database created before this change still carries the old grants.
--              New permissions approvals:read and masterdata:manage are added by the
--              seed itself when the backend starts; this script only REMOVES access.
-- Effect:      SS, RSM, GRSM, NSM and SD lose request:read and request:create (they
--              review requests inside Approvals); the "Cabang" label role, which is
--              not a login role, loses them too. Their request:approve is untouched.
--              Older roles outside the document (DEPARTMENT_APPROVER, FIELD_STAFF,
--              REGULAR_USER) are left as they are.
-- Apply manually, once, AFTER the backend with the new seed has started once:
--   psql "$DATABASE_URL" -1 -f migrations/000003_requests_access_per_matrix.sql
-- Safe to rerun: it only deletes rows that exist.

DELETE FROM role_permissions
WHERE role_id IN ('role_ss', 'role_rsm', 'role_grsm', 'role_nsm', 'role_sd', 'role_cabang')
  AND permission_id IN ('perm_req_read', 'perm_req_create');
