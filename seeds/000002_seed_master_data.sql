-- Seed: 000002_seed_master_data.sql
-- Description: Dummy development master data (regions, outlets, distributors and their
--              outlet mapping) taken from the SSAMS requirement mockups (Gambar 6-7).
-- Target: PostgreSQL (Supabase). Safe to rerun: every insert is skipped on conflict.
-- Not loaded by the application; apply manually:
--   psql "$DATABASE_URL" -1 -f seeds/000002_seed_master_data.sql
-- Requires the distributor_outlets table (created by database.MigrateAndSeed on server start).

INSERT INTO regions (id, code, name) VALUES
('region_west_java',    'WEST_JAVA',    'West Java'),
('region_east_java',    'EAST_JAVA',    'East Java'),
('region_north_sumatra','NORTH_SUMATRA','North Sumatra')
ON CONFLICT DO NOTHING;

INSERT INTO outlets (id, code, name, region_id) VALUES
('outlet_011', 'OUT-011', 'Bandung Kota',   (SELECT id FROM regions WHERE code = 'WEST_JAVA')),
('outlet_014', 'OUT-014', 'Surabaya Timur', (SELECT id FROM regions WHERE code = 'EAST_JAVA')),
('outlet_021', 'OUT-021', 'Medan Kota',     (SELECT id FROM regions WHERE code = 'NORTH_SUMATRA')),
('outlet_008', 'OUT-008', 'Bekasi Utara',   (SELECT id FROM regions WHERE code = 'WEST_JAVA')),
('outlet_030', 'OUT-030', 'Depok Tengah',   (SELECT id FROM regions WHERE code = 'WEST_JAVA')),
('outlet_025', 'OUT-025', 'Cirebon Kota',   (SELECT id FROM regions WHERE code = 'WEST_JAVA'))
ON CONFLICT DO NOTHING;

INSERT INTO distributors (id, code, name) VALUES
('dist_utama',   'DIST-001', 'PT Distributor Utama'),
('dist_selaras', 'DIST-002', 'PT Karya Selaras'),
('dist_mitra',   'DIST-003', 'PT Mitra Jaya Abadi'),
('dist_makmur',  'DIST-004', 'CV Sumber Makmur')
ON CONFLICT DO NOTHING;

-- One outlet is served by exactly one distributor.
INSERT INTO distributor_outlets (distributor_id, outlet_id)
SELECT d.id, o.id
FROM (VALUES
    ('DIST-001', 'OUT-011'), ('DIST-001', 'OUT-030'),
    ('DIST-002', 'OUT-014'), ('DIST-002', 'OUT-008'),
    ('DIST-003', 'OUT-021'),
    ('DIST-004', 'OUT-025')
) AS m(distributor_code, outlet_code)
JOIN distributors d ON d.code = m.distributor_code
JOIN outlets o ON o.code = m.outlet_code
ON CONFLICT DO NOTHING;
