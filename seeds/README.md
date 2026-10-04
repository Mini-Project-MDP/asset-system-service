# Database seeds

Store non-secret development and test reference data in this directory.

Seed files must be safe to rerun and must never contain production data,
credentials, tokens, or personal information.

## IMEI reference (Android fulfillment auto-fill)

`imei_reference` maps a TAC (the first 8 digits of an IMEI) to brand, model and
release year. The real source (GSMA TAC database, or the Asset Team's own
allocation data) is not in this repository.

| File | What it is |
|---|---|
| `000004_seed_imei_reference.sql` | 3 rows, dummy |
| `imei_reference_dummy.csv` | 24 dummy rows. TACs start with `99` on purpose so they never match a real device. Brands/models match the phone catalog (`000003`). |
| `imei_reference_template.csv` | Header and one example row for a real import |

Import a CSV (insert or update by TAC; `-dry-run` validates only):

```bash
go run ./cmd/importimei -file seeds/imei_reference_dummy.csv -dry-run
go run ./cmd/importimei -file seeds/imei_reference_dummy.csv
```

IMEIs to try in the Android fulfillment form (valid check digit):

| IMEI | Result |
|---|---|
| `990000011234566` | Samsung Galaxy Tab (2019) |
| `990000051234567` | Samsung A-series (2020) |
| `990000101234567` | Xiaomi Redmi Note (2019) |
| `990000151234566` | Oppo A-series (2020) |
| `990000191234568` | Vivo Standard (2021) |
| `990000221234562` | Realme Standard (2022) |
| `100000009999999` | not found (manual input) |

Remove the dummy rows when real data is loaded:

```sql
DELETE FROM imei_reference WHERE tac LIKE '99%';
```
