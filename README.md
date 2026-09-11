# Asset System Service

Backend Asset Management System menggunakan Go, Fiber, dan PostgreSQL (Supabase).

Saat ini tersedia: HTTP server, environment configuration, koneksi Supabase PostgreSQL, health check, readiness check, automated tests, serta skeleton modular untuk business feature. Endpoint bisnis belum diimplementasikan.

## Setup

### 1. Prerequisites

- Git
- Go sesuai versi pada `go.mod` (saat ini Go 1.27)

```powershell
git --version
go version
```

### 2. Clone dan install dependency

```powershell
git clone https://github.com/Mini-Project-MDP/asset-system-service.git
cd asset-system-service
go mod download
```

### 3. Siapkan `.env`

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
```

Isi credential development yang diberikan oleh tim:

```env
APP_ENV=development
APP_PORT=8080
DATABASE_PING_TIMEOUT=5s

DATABASE_URL=postgres://postgres.your-project-id:your-password@aws-0-region.pooler.supabase.com:5432/postgres?sslmode=require

# Approval Engine integration — lihat docs/approval-engine-integration-plan.md.
# api_key didapat dari POST /api/v1/applications di Approval-Engine-Service
# (ditampilkan sekali saat dibuat, simpan segera).
APPROVAL_ENGINE_BASE_URL=http://localhost:8000
APPROVAL_ENGINE_API_KEY=your-approval-engine-api-key
```

Jangan commit `.env` dan jangan mengirim token/api key melalui chat.

### Approval Engine integration

Sejak integrasi dengan `Approval-Engine-Service` (lihat
[docs/approval-engine-integration-plan.md](docs/approval-engine-integration-plan.md) dan
[docs/backend-milestones.md](docs/backend-milestones.md)), behavior berikut berubah:

- `POST /api/v1/requests` kini juga mendaftarkan request ke Approval Engine secara
  backend-to-backend. Kalau engine tidak bisa dihubungi, request tetap tersimpan lokal
  dengan `approval_status=PENDING_ENGINE_SYNC` (response ke user tetap sukses).
- `POST /api/v1/approvals/{id}/action` kini benar-benar memanggil keputusan approver ke
  engine; error bisnis dari engine (mis. bukan approver yang ditugaskan) diteruskan sebagai
  `400`.
- Jalankan `go run ./cmd/retrypendingsync` secara berkala (mis. via cron) untuk
  menyinkronkan ulang request yang sempat gagal terdaftar ke engine
  (`approval_status=PENDING_ENGINE_SYNC`). Tool ini keluar dengan exit code 1 kalau masih
  ada yang gagal, supaya wrapper cron bisa alert.
- Jalankan `go run ./cmd/syncparticipants` setelah ada perubahan data user/organisasi, supaya
  Approval Engine tahu struktur atasan-bawahan terbaru untuk resolusi approver.

**Rotasi `APPROVAL_ENGINE_API_KEY`:** `api_key` hanya ditampilkan sekali saat
`POST /api/v1/applications` dipanggil di `Approval-Engine-Service` — tidak bisa diambil ulang.
Untuk rotasi: registrasikan ulang application (`code` boleh sama, akan dapat `api_key` baru),
update `APPROVAL_ENGINE_API_KEY` di `.env`/secret manager deployment, lalu restart service.
Key lama tetap valid sampai application lama di-nonaktifkan secara manual di sisi engine
(belum ada endpoint deaktivasi application per key — dicatat sebagai gap di plan
`Approval-Engine-Service`).

### 4. Test dan jalankan

```powershell
go test ./...
go vet ./...
go run ./cmd/api
```

Aplikasi akan berhenti jika konfigurasi tidak valid atau database tidak dapat dihubungi.

### 5. Verifikasi

Gunakan port sesuai `APP_PORT`:

```powershell
Invoke-RestMethod http://localhost:8080/health
Invoke-RestMethod http://localhost:8080/ready
```

Expected `/ready` response:

```json
{
  "database": "connected",
  "service": "asset-system-service",
  "status": "ready"
}
```

Hentikan server dengan `Ctrl+C`.

## Configuration

| Variable | Wajib | Default | Fungsi |
|---|---:|---:|---|
| `APP_ENV` | Tidak | `development` | Nama environment |
| `APP_PORT` | Tidak | `8080` | Port API |
| `DATABASE_PING_TIMEOUT` | Tidak | `5s` | Timeout database ping |
| `DATABASE_URL` | Ya | - | URL database PostgreSQL / Supabase |
| `DB_HOST` | Opsional | - | Host Supabase/PostgreSQL alternatif |
| `DB_PORT` | Opsional | `5432` | Port database |
| `DB_USER` | Opsional | `postgres` | User database |
| `DB_PASSWORD` | Opsional | - | Password database |
| `DB_NAME` | Opsional | `postgres` | Nama database |
| `DB_SSLMODE` | Opsional | `require` | SSL mode koneksi database |

Environment variable dari sistem/hosting memiliki prioritas lebih tinggi daripada `.env`.

## Endpoints

| Method | Endpoint | Fungsi |
|---|---|---|
| `GET` | `/health` | Memastikan API hidup |
| `GET` | `/ready` | Memastikan API dan Database siap |

`/ready` menghasilkan `503 Service Unavailable` jika database tidak dapat dihubungi.

## Project Structure

```text
asset-system-service/
├── cmd/api/                         # Entry point; bootstrap dan shutdown
├── pkg/                             # Shared packages, delivery, domain, repo, and services
├── migrations/                      # Perubahan schema SQL berurutan
├── seeds/                           # Data referensi development/test
├── docs/api/                        # API contract
├── docs/design/                     # ERD dan desain teknis
├── .env.example
├── go.mod
├── go.sum
└── README.md
```

Folder `cmd` adalah konvensi Go untuk aplikasi yang dapat dijalankan, bukan Windows Command Prompt.

Project menggunakan modular monolith. Alur kode yang dituju:

```text
HTTP Handler → Application Service → Domain → Repository → Turso
```

Business logic tidak boleh diletakkan di handler atau bergantung langsung pada Gin/Turso.

## Development Commands

| Tujuan | Perintah |
|---|---|
| Run API | `go run ./cmd/api` |
| Format | `go fmt ./...` |
| Test | `go test ./...` |
| Static check | `go vet ./...` |
| Download dependency | `go mod download` |
| Rapikan dependency | `go mod tidy` |

Sebelum pull request, jalankan `go fmt ./...`, `go test ./...`, dan `go vet ./...`.

## Troubleshooting

### `go` tidak dikenali

Tutup seluruh VS Code lalu buka kembali. Pastikan `C:\Program Files\Go\bin` tersedia pada System `PATH`.

```powershell
& "C:\Program Files\Go\bin\go.exe" version
```

### Port sudah digunakan

```powershell
Get-NetTCPConnection -LocalPort 8080 -State Listen
```

Ganti port melalui `.env`, misalnya `APP_PORT=8081`, lalu jalankan ulang API.

### Database gagal terhubung

Pastikan `.env` berada sejajar dengan `go.mod`, URL dimulai dengan `libsql://`, dan token masih aktif. Jangan sertakan token saat membagikan error.

### `go.mod file not found`

Terminal tidak berada di root repository. Masuk ke folder `asset-system-service`, lalu jalankan kembali perintahnya.

## Next

Tahap berikutnya: sepakati bagian **Keputusan Terbuka** pada API contract, lalu buat initial migration dan implementasikan `masterdata` sebagai pola modul pertama.

> Driver `libsql-client-go` digunakan agar development berjalan pada Windows dan perlu dievaluasi kembali sebelum deployment production.
