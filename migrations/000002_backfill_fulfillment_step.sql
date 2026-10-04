-- One-off data fix: 000002_backfill_fulfillment_step.sql
-- Description: Requests approved before the Processing step was introduced have
--              fulfillment_step = NULL, so they never reach the Asset Team's
--              fulfillment queue. New approvals now set step 0 themselves
--              (SetApprovalDecisionResult); this brings the older rows in line.
-- Apply manually, once:  psql "$DATABASE_URL" -1 -f migrations/000002_backfill_fulfillment_step.sql
-- Safe to rerun: it only touches approved requests that still have no step.

UPDATE asset_requests
SET fulfillment_step = 0, updated_at = CURRENT_TIMESTAMP
WHERE status = 'APPROVED' AND fulfillment_step IS NULL;

-- Optional, review before running: fulfillment_data written before it was stored as JSON
-- looks like 'map[codes:[A B]]' (Go formatting) and cannot be converted back reliably.
-- The API already reports such data as absent. To clear it so the request can be re-entered:
--   SELECT id, status, fulfillment_step, left(fulfillment_data, 60) FROM asset_requests
--   WHERE fulfillment_data IS NOT NULL AND fulfillment_data NOT LIKE '{%';
-- (only requests still at step 0 can be re-entered; later ones keep their stage.)
