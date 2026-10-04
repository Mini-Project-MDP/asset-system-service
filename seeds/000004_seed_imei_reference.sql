-- Seed: 000004_seed_imei_reference.sql
-- Description: Dummy development IMEI reference (TAC -> brand/model/release year), taken
--              from the three IMEIs that were hardcoded in the frontend. The real source
--              is the GSMA TAC database, which is licensed and not included here.
-- Target: PostgreSQL (Supabase). Safe to rerun.
-- Not loaded by the application; apply manually:
--   psql "$DATABASE_URL" -1 -f seeds/000004_seed_imei_reference.sql
-- Requires the imei_reference table (created by database.MigrateAndSeed on server start).

INSERT INTO imei_reference (tac, brand, model, release_year) VALUES
('35489201', 'Samsung', 'Galaxy Tab', '2023'),
('86492019', 'Xiaomi',  'Redmi Note', '2022'),
('35829102', 'Oppo',    'A-series',   '2024')
ON CONFLICT DO NOTHING;
