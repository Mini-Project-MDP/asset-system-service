-- Seed: 000003_seed_phone_catalog.sql
-- Description: Dummy development phone brand/model catalog for the Android fulfillment
--              form, taken from the lists that were hardcoded in the frontend.
--              The brand->model pairing is an assumption: the old frontend list was flat.
-- Target: PostgreSQL (Supabase). Safe to rerun: every insert is skipped on conflict.
-- Not loaded by the application; apply manually:
--   psql "$DATABASE_URL" -1 -f seeds/000003_seed_phone_catalog.sql
-- Requires the phone_brands / phone_models tables (created by database.MigrateAndSeed on server start).

INSERT INTO phone_brands (id, name) VALUES
('brand_samsung', 'Samsung'),
('brand_xiaomi',  'Xiaomi'),
('brand_oppo',    'Oppo'),
('brand_vivo',    'Vivo'),
('brand_realme',  'Realme')
ON CONFLICT DO NOTHING;

INSERT INTO phone_models (id, brand_id, name)
SELECT m.id, b.id, m.name
FROM (VALUES
    ('model_samsung_galaxy_tab', 'Samsung', 'Galaxy Tab'),
    ('model_samsung_a_series',   'Samsung', 'A-series'),
    ('model_xiaomi_redmi_note',  'Xiaomi',  'Redmi Note'),
    ('model_oppo_a_series',      'Oppo',    'A-series'),
    ('model_vivo_standard',      'Vivo',    'Standard'),
    ('model_realme_standard',    'Realme',  'Standard')
) AS m(id, brand_name, name)
JOIN phone_brands b ON b.name = m.brand_name
ON CONFLICT DO NOTHING;
