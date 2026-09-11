# Instruksi eksekusi AI — Sambungkan `asset-system-service` ↔ `Approval-Engine-Service`

**Untuk siapa dokumen ini:** agent AI (mis. Claude Code) yang dijalankan di working directory
`asset-system-service`, dengan akses ke repo ini **dan** repo `Approval-Engine-Service` (biasanya
sibling folder, `../Approval-Engine-Service` atau `../Approval engine/Approval-Engine-Service`).

**Status sebelum dokumen ini ditulis (sudah diverifikasi, jangan diulang):**
- Kedua repo `go build ./...`, `go vet ./...`, `go test ./...` bersih.
- Kontrak webhook sudah cocok 100% (`EngineWebhookEvent` di asset-system field-per-field sama
  dengan `WebhookEvent` di engine; signature HMAC-SHA256 `sha256=<hex>` dengan `api_key` sebagai
  key sama persis di kedua sisi).
- `POST /requests/:id/decision` di engine tidak butuh `X-API-Key` — tidak ada perubahan kontrak
  yang perlu disesuaikan di klien asset-system.
- `asset-system-service/.env.example` sudah diperbarui, memuat `APPROVAL_ENGINE_BASE_URL`,
  `APPROVAL_ENGINE_API_KEY`, `ALLOWED_ORIGINS`.

**Yang belum diverifikasi dan jadi tugas dokumen ini:** apakah kedua sistem benar-benar bisa
saling bicara di environment nyata (bukan cuma kompatibel secara kode).

---

## Sebelum mulai — WAJIB dapat input ini dari manusia dulu

Jangan menebak nilai-nilai ini. Tanyakan eksplisit sebelum eksekusi Task manapun di bawah:

1. **`ENGINE_BASE_URL`** — URL Approval-Engine-Service yang hidup (mis. `https://approval-engine.vercel.app`).
   Tidak bisa ditebak dari kode; ini keputusan deployment.
2. **Apakah `Application` untuk asset-system sudah pernah didaftarkan di engine ini sebelumnya?**
   Kalau ya, minta `api_key`-nya sekarang — endpoint `POST /applications` cuma menampilkan
   `api_key` **sekali** saat dibuat, tidak bisa diambil ulang lewat API apa pun. Kalau key itu
   hilang, satu-satunya jalan adalah registrasi ulang dengan `code` baru (lihat catatan di
   Task 2 soal konsekuensinya).
3. **`ASSET_SYSTEM_PUBLIC_URL`** — URL publik `asset-system-service` sendiri yang bisa dituju
   engine untuk mengirim webhook (mis. `https://asset-system-service.vercel.app`). Kalau belum
   ada deployment publik (masih localhost), lewati Task 5 dan catat sebagai belum bisa diverifikasi.

Kalau salah satu di atas tidak bisa didapat, **hentikan eksekusi di titik itu** dan laporkan ke
manusia — jangan lanjut dengan asumsi/nilai palsu.

---

## Task 1 — Pastikan kode di kedua repo dalam kondisi bersih

```bash
cd Approval-Engine-Service && go build ./... && go vet ./... && go test ./...
cd ../asset-system-service && go build ./... && go vet ./... && go test ./...
```

**Sukses kalau:** kedua perintah keluar tanpa error, semua `ok` di test.
**Kalau gagal:** hentikan, laporkan output error apa adanya — jangan coba perbaiki kode di luar
scope dokumen ini.

---

## Task 2 — Pastikan `Application` untuk asset-system terdaftar di engine

Cek dulu apakah sudah ada (endpoint ini tidak mengembalikan `api_key`, aman dipanggil berkali-kali):

```bash
curl -s "$ENGINE_BASE_URL/api/v1/applications?limit=50" | jq '.data.items[] | {id, code, name, callback_url, is_active}'
```

**Kalau sudah ada** entry dengan `code` yang jelas ini punya asset-system (mis. `assetmgmt`):
gunakan `api_key` yang sudah didapat dari manusia di langkah pra-syarat. **Jangan** registrasi
ulang dengan `code` yang sama — endpoint ini tidak melakukan upsert, `code` harus unik, registrasi
ulang dengan `code` sama akan gagal karena constraint unique.

**Kalau belum ada**, registrasikan:

```bash
curl -s -X POST "$ENGINE_BASE_URL/api/v1/applications" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "assetmgmt",
    "name": "Asset Management System",
    "callback_url": "'"$ASSET_SYSTEM_PUBLIC_URL"'/api/v1/webhooks/approval-engine"
  }' | jq .
```

**Simpan `data.api_key` dari response ini SEKARANG** ke `.env` asset-system (Task 3) — tidak akan
muncul lagi setelah ini.

**Batasan yang perlu dilaporkan, bukan diakali:** tidak ada endpoint update untuk `Application`
di engine saat ini (cuma `Create` + `List`). Kalau app sudah terdaftar TANPA `callback_url` dan
perlu ditambahkan sekarang, **tidak bisa** lewat API — itu perlu perubahan kode di
`Approval-Engine-Service` (endpoint `PATCH /applications/:id`, belum ada) atau update manual di
database. Jangan registrasi ulang dengan `code` baru hanya demi menambahkan `callback_url` —
itu mengganti `api_key` yang sudah dipakai integrasi berjalan. Laporkan ke manusia sebagai item
terpisah.

---

## Task 3 — Isi environment variable asset-system-service

Di `.env` asset-system-service (buat dari `.env.example` kalau belum ada):

```env
APPROVAL_ENGINE_BASE_URL=$ENGINE_BASE_URL
APPROVAL_ENGINE_API_KEY=<api_key dari Task 2>
```

**Jangan commit file `.env` ini.** Verifikasi `.gitignore` sudah mengecualikannya:

```bash
git check-ignore -v .env
```

**Sukses kalau:** command di atas menampilkan baris yang mengecualikan `.env` (exit code 0).
**Kalau tidak ter-ignore:** hentikan, laporkan — jangan lanjut sampai ini beres, supaya `api_key`
tidak ter-commit.

---

## Task 4 — Sinkronkan participant ke engine

```bash
go run ./cmd/syncparticipants
```

Perintah ini baca `TURSO_DATABASE_URL`/`TURSO_AUTH_TOKEN` (data asset-system sendiri) dan
`APPROVAL_ENGINE_BASE_URL`/`APPROVAL_ENGINE_API_KEY` (dari Task 3), lalu push semua
user+org-chart ke `POST /participants/import` di engine.

**Sukses kalau:** log akhir `"synced N participant(s) to Approval-Engine-Service"`, N > 0.
**Kalau `"no participants found"`:** database asset-system yang dipakai kosong — bukan masalah
koneksi ke engine, laporkan sebagai isu data lokal, bukan isu integrasi.
**Kalau gagal koneksi ke engine** (`"import participants: engine responded ..."` atau timeout):
ini baru genuinely masalah integrasi — cek ulang `ENGINE_BASE_URL` benar dan reachable dari
environment yang menjalankan perintah ini.

---

## Task 5 — Verifikasi workflow yang dibutuhkan sudah aktif di engine

```bash
curl -s "$ENGINE_BASE_URL/api/v1/workflows?app_id=assetmgmt" | jq '.data.items[] | {doc_type, version, is_active}'
```

`asset-system-service` butuh workflow aktif untuk **kedua** `doc_type` ini (dari
`internal/pkg/service/request_service.go`, fungsi `approvalEngineDocType`):
- `asset_request_barcode`
- `asset_request_field_device`

**Kalau keduanya sudah muncul dengan `is_active: true`:** lanjut ke Task 6.

**Kalau salah satu/keduanya belum ada:** **JANGAN membuat workflow sendiri lewat
`POST /workflows`.** Struktur approval chain (siapa approver tiap tahap, berapa level, mode
any/all) adalah keputusan bisnis yang harus ditentukan tim yang punya konteks proses persetujuan
asset — bukan sesuatu yang boleh AI tebak dan publish ke engine yang datanya dipakai sungguhan.
Laporkan ke manusia: `doc_type` mana yang belum ada workflow aktifnya, minta didesain lewat
Approval-Engine-Client (halaman Administrasi → Workflow) atau berikan spesifikasinya untuk
dieksekusi terpisah.

---

## Task 6 — Uji coba end-to-end (kalau Task 2–5 semua lolos)

```bash
curl -s -X POST "$ENGINE_BASE_URL/api/v1/requests" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $APPROVAL_ENGINE_API_KEY" \
  -d '{
    "doc_type": "asset_request_barcode",
    "resource_id": "AI-VERIFY-0001",
    "requester_id": "<NIK yang sudah pasti ada dari Task 4>",
    "payload": { "requesterApprovalRank": 10 }
  }' | jq .
```

**Sukses kalau:** response `"success": true`, ada `data.id` dan `data.steps` berisi minimal satu
step. Catat `data.id` ini.

**Kalau `"requester ... is not a registered participant"`:** Task 4 belum lengkap atau NIK yang
dipakai salah — bukan masalah koneksi.

**Kalau `"no active workflow for app ... doc_type ..."`:** Task 5 sebenarnya belum lolos, kembali
ke situ.

**Kalau lolos:** ambil `id` dari response, cek statusnya:

```bash
curl -s "$ENGINE_BASE_URL/api/v1/requests/<id dari atas>" | jq '{status, current_step_order, steps}'
```

Ini adalah bukti konkret kedua sistem **sudah benar-benar terhubung**, bukan cuma kompatibel di
kode. Request uji coba ini boleh dibiarkan (statusnya akan `pending` menunggu approval sungguhan,
tidak mengganggu data lain) atau diberi tahu ke tim workflow supaya tidak bingung melihat 1 baris
data uji di dashboard.

---

## Ringkasan yang harus dilaporkan di akhir

Setelah semua task dijalankan (atau berhenti di satu titik), laporkan dalam bentuk daftar:

- [ ] Task 1 — build/test kedua repo bersih
- [ ] Task 2 — Application terdaftar, `api_key` didapat, `callback_url` terisi (atau: gap
      terlapor kalau `callback_url` tidak bisa ditambahkan ke app yang sudah ada)
- [ ] Task 3 — `.env` terisi, `.gitignore` mengecualikannya
- [ ] Task 4 — participant tersinkron, jumlah N
- [ ] Task 5 — kedua `doc_type` punya workflow aktif (atau: daftar `doc_type` yang belum, untuk
      dieskalasi)
- [ ] Task 6 — satu request uji coba berhasil dibuat & terbaca statusnya

Task yang tidak lolos karena butuh keputusan manusia (Task 2 update callback_url, Task 5 desain
workflow) **bukan kegagalan integrasi** — laporkan sebagai item terbuka yang jelas, jangan
diakali dengan asumsi.
