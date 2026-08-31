# Asset Management System - API Contract

**Version:** 1.0  
**Status:** Draft for implementation  
**Base path:** `/api/v1`  
**Format:** JSON  
**Time format:** ISO 8601 UTC, contoh `2026-08-30T16:30:00Z`

## 1. Scope

Contract ini mencakup API untuk:

- dashboard dan global search;
- digital asset request;
- reusable approval routing;
- fulfillment;
- asset registry dan assignment;
- master data;
- notification dan audit trail.

Authentication diasumsikan berasal dari corporate SSO/JWT. Implementasi login dan penerbitan token berada di luar scope contract v1.

## 2. Keputusan Domain

1. Satu request hanya memiliki satu asset category: `BARCODE`, `ANDROID`, atau `SERVER`.
2. `request_type_id` hanya wajib untuk category yang mengaktifkannya, saat ini Android.
3. Outlet harus termasuk mapping distributor yang dipilih.
4. Approval route dihasilkan dari workflow aktif untuk category request.
5. Hanya role dengan rank di atas requester role yang menjadi approver.
6. Jika requester berada pada level tertinggi, request otomatis `APPROVED` dan masuk fulfillment.
7. Approval berjalan sequential: satu task `PENDING`, task berikutnya `WAITING`.
8. `REJECT` bersifat terminal untuk workflow instance tersebut.
9. `REQUEST_REVISION` mengembalikan request ke requester. Saat resubmit, approval kembali ke approver yang meminta revisi dan approval sebelumnya tetap valid.
10. Jika field penentu route berubah ketika revisi (`asset_type`, `requester_role`, distributor/outlet, atau sales division), workflow lama dibatalkan dan route dibuat ulang.
11. Fulfillment dibuat otomatis setelah request approved.
12. Fulfillment dan approval menggunakan state machine terpisah.
13. Android membutuhkan IMEI per unit; Server membutuhkan serial number per unit; Barcode tidak membutuhkan identifier.
14. Fulfillment hanya dapat diselesaikan jika jumlah item sama dengan quantity request dan seluruh identifier wajib valid.
15. Semua perubahan status dan keputusan disimpan sebagai immutable history/audit event.

## 3. Conventions

### 3.1 Authentication

Semua endpoint `/api/v1` selain `/health` dan `/ready` membutuhkan:

```http
Authorization: Bearer <access-token>
```

### 3.2 Headers

```http
Content-Type: application/json
Accept: application/json
X-Request-ID: optional-client-correlation-id
Idempotency-Key: required-for-create-and-action-commands
```

`Idempotency-Key` wajib untuk create request, approval action, fulfillment transition, asset registration, dan assignment.

### 3.3 ID dan Version

- Primary key API menggunakan UUID string.
- Nomor bisnis seperti `REQ-2093` dan `FUL-1021` dibuat server.
- Resource mutable memiliki integer `version` untuk optimistic locking.
- Client mengirim `version` terakhir pada update/action. Version stale menghasilkan `409 VERSION_CONFLICT`.

### 3.4 Success Envelope

```json
{
  "data": {},
  "meta": {
    "request_id": "01J6..."
  }
}
```

List response:

```json
{
  "data": [],
  "meta": {
    "page": 1,
    "page_size": 20,
    "total_items": 120,
    "total_pages": 6,
    "request_id": "01J6..."
  }
}
```

### 3.5 Error Envelope

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request payload is invalid",
    "details": [
      {
        "field": "quantity",
        "reason": "must be greater than zero"
      }
    ]
  },
  "meta": {
    "request_id": "01J6..."
  }
}
```

Common status:

| HTTP | Code | Penggunaan |
|---:|---|---|
| 400 | `VALIDATION_ERROR` | Payload/query tidak valid |
| 401 | `UNAUTHENTICATED` | Token tidak ada/invalid |
| 403 | `FORBIDDEN` | Role tidak memiliki akses |
| 404 | `RESOURCE_NOT_FOUND` | Resource tidak ditemukan |
| 409 | `VERSION_CONFLICT` | Optimistic lock gagal |
| 409 | `INVALID_STATE_TRANSITION` | Action tidak valid pada status sekarang |
| 409 | `DUPLICATE_IDENTIFIER` | IMEI/serial/barcode sudah terdaftar |
| 422 | `BUSINESS_RULE_VIOLATION` | Business rule gagal |
| 429 | `RATE_LIMITED` | Terlalu banyak request |
| 500 | `INTERNAL_ERROR` | Error internal tanpa detail sensitif |
| 503 | `DEPENDENCY_UNAVAILABLE` | Database/dependency tidak tersedia |

### 3.6 Pagination dan Sorting

Standard query:

```text
page=1
page_size=20
sort=-created_at
```

- `page_size`: 1-100.
- Prefix `-` berarti descending.
- Invalid sort field menghasilkan `400 VALIDATION_ERROR`.

## 4. Roles dan Access

| Capability | Requester | Approver | Asset Team | Admin | Auditor |
|---|:---:|:---:|:---:|:---:|:---:|
| Create request | Yes | Yes | Yes | Yes | No |
| View own request | Yes | Yes | Yes | Yes | Read only |
| View all request | No | Scoped | Yes | Yes | Read only |
| Approve/revise/reject | Assigned only | Assigned only | If assigned | Yes | No |
| Process fulfillment | No | No | Yes | Yes | No |
| Manage assets | No | No | Yes | Yes | Read only |
| Manage master/workflow | No | No | No | Yes | Read only |
| View audit | No | No | Scoped | Yes | Yes |

Actual authorization wajib menggunakan user identity dari token, bukan `user_id` yang dikirim client.

## 5. Enumerations

### 5.1 Request

```text
priority: NORMAL | HIGH | URGENT

request_status:
SUBMITTED | IN_APPROVAL | REVISION_REQUIRED | APPROVED |
REJECTED | IN_FULFILLMENT | COMPLETED | CANCELLED
```

### 5.2 Workflow dan Approval

```text
workflow_status: ACTIVE | INACTIVE | RETIRED
workflow_instance_status: RUNNING | REVISION_REQUIRED | APPROVED | REJECTED | CANCELLED

approval_task_status:
WAITING | PENDING | APPROVED | REJECTED |
REVISION_REQUESTED | SKIPPED | CANCELLED

approval_action:
SUBMITTED | ACTIVATED | APPROVED | REJECTED |
REVISION_REQUESTED | RESUBMITTED | SKIPPED | AUTO_APPROVED | CANCELLED
```

### 5.3 Fulfillment

```text
fulfillment_status:
PENDING | PROCESSING | SHIPPED | DELIVERED | INSTALLED | COMPLETED | CANCELLED

fulfillment_item_status:
PENDING | PREPARED | SHIPPED | DELIVERED | REGISTERED | COMPLETED | CANCELLED
```

### 5.4 Asset

```text
identifier_type: NONE | IMEI | SERIAL_NUMBER | BARCODE

asset_status:
REGISTERED | IN_STOCK | ASSIGNED | IN_USE |
MAINTENANCE | RETURNED | RETIRED | DISPOSED
```

## 6. Core Resource Shapes

### 6.1 Asset Request

| Field | Type | Nullable | Notes |
|---|---|:---:|---|
| `id` | UUID | No | Internal ID |
| `request_no` | string | No | Human-readable, server generated |
| `asset_type` | object | No | Category Barcode/Android/Server |
| `request_type` | object | Yes | Wajib sesuai konfigurasi asset type |
| `distributor` | object | No | Distributor terpilih |
| `outlet` | object | No | Harus ter-cover distributor |
| `sales_division` | object | No | Sales division |
| `requester` | object | No | User/name dan role requester |
| `created_by` | object | No | User login pembuat record |
| `quantity` | integer | No | Minimal 1 |
| `priority` | enum | No | Default `NORMAL` |
| `status` | enum | No | Request status |
| `revision_no` | integer | No | Mulai dari 0 |
| `notes` | string | Yes | Maksimum 1000 karakter |
| `version` | integer | No | Optimistic locking |
| `submitted_at` | datetime | No | Submission time |
| `approved_at` | datetime | Yes | Final approval time |
| `completed_at` | datetime | Yes | Fulfillment completion time |

### 6.2 Approval Task

| Field | Type | Notes |
|---|---|---|
| `id` | UUID | Task ID |
| `request_id` | UUID | Asset request reference |
| `request_no` | string | Human-readable request number |
| `sequence_no` | integer | Urutan route |
| `role` | object | Required approver role |
| `assigned_to` | object/null | Specific user jika sudah resolved |
| `status` | enum | Current task status |
| `version` | integer | Optimistic locking |
| `activated_at` | datetime/null | Saat menjadi pending |
| `acted_at` | datetime/null | Saat keputusan dibuat |

### 6.3 Fulfillment

| Field | Type | Notes |
|---|---|---|
| `id` | UUID | Fulfillment ID |
| `fulfillment_no` | string | Server generated |
| `request` | object | Request summary |
| `assigned_to` | object/null | Asset team PIC |
| `status` | enum | Fulfillment status |
| `items` | array | Satu item per unit |
| `tracking_no` | string/null | Nomor pengiriman |
| `version` | integer | Optimistic locking |
| `started_at` | datetime/null | Processing start |
| `completed_at` | datetime/null | Completion time |

### 6.4 Asset

| Field | Type | Notes |
|---|---|---|
| `id` | UUID | Asset ID |
| `asset_code` | string | Unique internal code |
| `asset_type` | object | Asset category |
| `identifiers` | array | IMEI/serial/barcode |
| `status` | enum | Asset lifecycle status |
| `outlet` | object | Current location |
| `holder` | object/null | Current assigned user |
| `source_request` | object/null | Request asal |
| `version` | integer | Optimistic locking |

## 7. Endpoint Catalog

### 7.1 System, Profile, Search, Dashboard

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET | `/health` | Public | Liveness |
| GET | `/ready` | Public | Database readiness |
| GET | `/api/v1/me` | Authenticated | Current user/profile/roles |
| GET | `/api/v1/search` | Authenticated | Search request, outlet, asset identifier |
| GET | `/api/v1/dashboard/summary` | Authenticated | Dashboard cards |
| GET | `/api/v1/dashboard/request-trends` | Authenticated | Monthly category trends |
| GET | `/api/v1/activities` | Authenticated | Recent activity |

### 7.2 Requests

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/approval-routes/preview` | Preview route before submit |
| GET | `/api/v1/requests` | List/filter requests |
| POST | `/api/v1/requests` | Create and submit request |
| GET | `/api/v1/requests/{request_id}` | Request detail and tracking |
| PATCH | `/api/v1/requests/{request_id}` | Edit revision-required request |
| POST | `/api/v1/requests/{request_id}/resubmit` | Resubmit after revision |
| POST | `/api/v1/requests/{request_id}/cancel` | Cancel eligible request |
| GET | `/api/v1/requests/{request_id}/timeline` | Combined request timeline |

### 7.3 Approval

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/approval-tasks` | My queue/history |
| GET | `/api/v1/approval-tasks/{task_id}` | Approval detail |
| POST | `/api/v1/approval-tasks/{task_id}/approve` | Approve current task |
| POST | `/api/v1/approval-tasks/{task_id}/request-revision` | Request revision |
| POST | `/api/v1/approval-tasks/{task_id}/reject` | Reject request |
| GET | `/api/v1/workflow-instances/{instance_id}` | Runtime workflow detail |
| GET | `/api/v1/workflow-instances/{instance_id}/actions` | Immutable approval history |

### 7.4 Fulfillment

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/fulfillments` | Fulfillment queue/history |
| GET | `/api/v1/fulfillments/{fulfillment_id}` | Fulfillment detail |
| POST | `/api/v1/fulfillments/{fulfillment_id}/start` | Start processing |
| POST | `/api/v1/fulfillments/{fulfillment_id}/items` | Add/register unit |
| PATCH | `/api/v1/fulfillments/{fulfillment_id}/items/{item_id}` | Update unit/identifier |
| POST | `/api/v1/fulfillments/{fulfillment_id}/ship` | Mark shipped |
| POST | `/api/v1/fulfillments/{fulfillment_id}/deliver` | Mark delivered/installed |
| POST | `/api/v1/fulfillments/{fulfillment_id}/complete` | Complete fulfillment |
| GET | `/api/v1/fulfillments/{fulfillment_id}/events` | Fulfillment timeline |

### 7.5 Assets

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/assets` | Asset registry list |
| POST | `/api/v1/assets` | Manual asset registration |
| GET | `/api/v1/assets/{asset_id}` | Asset detail |
| PATCH | `/api/v1/assets/{asset_id}` | Update asset metadata/status |
| POST | `/api/v1/assets/{asset_id}/assignments` | Assign asset |
| POST | `/api/v1/assets/{asset_id}/return` | Return asset |
| GET | `/api/v1/assets/{asset_id}/history` | Asset lifecycle history |

### 7.6 Notifications dan Audit

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/notifications` | Notification inbox |
| PATCH | `/api/v1/notifications/{notification_id}/read` | Mark one as read |
| POST | `/api/v1/notifications/read-all` | Mark all as read |
| GET | `/api/v1/audit-logs` | Admin/auditor audit query |

### 7.7 Master Data dan Workflow

| Resource | Base Path |
|---|---|
| Regions | `/api/v1/regions` |
| Outlets | `/api/v1/outlets` |
| Distributors | `/api/v1/distributors` |
| Distributor-outlet mapping | `/api/v1/distributors/{id}/outlets` |
| Sales divisions | `/api/v1/sales-divisions` |
| Asset types | `/api/v1/asset-types` |
| Request types | `/api/v1/request-types` |
| Users | `/api/v1/users` |
| Roles | `/api/v1/roles` |
| Workflow definitions | `/api/v1/workflow-definitions` |

Master resources mendukung `GET list`, `POST create`, `GET /{id}`, `PATCH /{id}`, dan `DELETE /{id}` sebagai soft deactivate. Workflow menggunakan explicit activate/retire actions.

## 8. Detailed Contracts

### 8.1 Current User

```http
GET /api/v1/me
```

```json
{
  "data": {
    "id": "usr_01",
    "employee_no": "EMP00123",
    "name": "Andi P.",
    "email": "andi@company.co",
    "roles": [
      {"id": "role_admin", "code": "ADMIN", "name": "Admin"}
    ],
    "permissions": ["request:create", "master:manage", "audit:read"]
  }
}
```

### 8.2 Global Search

```http
GET /api/v1/search?q=REQ-2093&limit=10
```

Search fields: request number, outlet code/name, asset code, IMEI, serial number, and barcode.

```json
{
  "data": [
    {
      "type": "REQUEST",
      "id": "req_uuid",
      "label": "REQ-2093",
      "description": "Barcode - Cirebon Kota",
      "url": "/requests/req_uuid"
    }
  ]
}
```

### 8.3 Dashboard Summary

```http
GET /api/v1/dashboard/summary
```

```json
{
  "data": {
    "total_requests": 12,
    "pending_approvals": 5,
    "fulfillments_in_progress": 3,
    "assets_registered": 1247
  }
}
```

```http
GET /api/v1/dashboard/request-trends?year=2026
```

```json
{
  "data": {
    "year": 2026,
    "series": [
      {"category": "BARCODE", "monthly_counts": [4, 7, 5, 9, 4, 8, 7, 6, 0, 0, 0, 0]},
      {"category": "ANDROID", "monthly_counts": [2, 3, 4, 2, 5, 3, 4, 6, 0, 0, 0, 0]},
      {"category": "SERVER", "monthly_counts": [1, 1, 2, 1, 0, 2, 1, 2, 0, 0, 0, 0]}
    ]
  }
}
```

### 8.4 Preview Approval Route

```http
POST /api/v1/approval-routes/preview
```

```json
{
  "asset_type_id": "asset_type_barcode",
  "requester_role_id": "role_rsm",
  "distributor_id": "dist_01",
  "outlet_id": "outlet_01",
  "sales_division_id": "division_01"
}
```

```json
{
  "data": {
    "workflow_definition_id": "wf_barcode_v1",
    "workflow_code": "BARCODE_APPROVAL",
    "auto_approved": false,
    "steps": [
      {"sequence_no": 1, "role_code": "GRSM", "role_name": "Group Regional Sales Manager"},
      {"sequence_no": 2, "role_code": "NSM", "role_name": "National Sales Manager"},
      {"sequence_no": 3, "role_code": "SD", "role_name": "Sales Director"}
    ]
  }
}
```

Errors: `OUTLET_NOT_COVERED`, `WORKFLOW_NOT_CONFIGURED`, `REQUESTER_ROLE_NOT_IN_FLOW`.

### 8.5 Create Request

```http
POST /api/v1/requests
Idempotency-Key: 0872be58-0db4-4ba3-aed4-cbf853ca8cb6
```

```json
{
  "asset_type_id": "asset_type_android",
  "request_type_id": "request_type_new_device",
  "distributor_id": "dist_01",
  "outlet_id": "outlet_08",
  "sales_division_id": "division_m1_bis",
  "requester_user_id": "usr_laras",
  "requester_role_id": "role_sales_admin",
  "requester_name": "Laras P.",
  "quantity": 5,
  "priority": "HIGH",
  "notes": "Device untuk outlet baru"
}
```

`created_by` selalu diambil dari access token. `requester_user_id` boleh null jika requester belum terdaftar; dalam kondisi tersebut `requester_name` wajib.

Response: `201 Created`

```json
{
  "data": {
    "id": "req_uuid",
    "request_no": "REQ-2094",
    "status": "IN_APPROVAL",
    "version": 1,
    "approval": {
      "instance_id": "wfi_uuid",
      "current_role": {"code": "CABANG", "name": "Cabang / Distributor"},
      "steps_total": 4
    },
    "submitted_at": "2026-08-30T16:30:00Z"
  }
}
```

### 8.6 List Requests

```http
GET /api/v1/requests?status=IN_APPROVAL&asset_type=BARCODE&outlet_id=outlet_01&search=REQ-20&page=1&page_size=20&sort=-submitted_at
```

Supported filters:

- `status` (repeatable);
- `asset_type_id` or `asset_type` code;
- `distributor_id`;
- `outlet_id`;
- `sales_division_id`;
- `requester_user_id`;
- `priority`;
- `attention_required=true`;
- `submitted_from`, `submitted_to`;
- `search`.

Each list item includes request number, type, outlet, quantity, requester, priority, status, current approval step, and timestamps.

### 8.7 Request Detail

```http
GET /api/v1/requests/{request_id}
```

```json
{
  "data": {
    "id": "req_uuid",
    "request_no": "REQ-2093",
    "asset_type": {"id": "asset_type_barcode", "code": "BC", "name": "Barcode"},
    "request_type": null,
    "distributor": {"id": "dist_04", "name": "CV Sumber Makmur"},
    "outlet": {"id": "outlet_25", "code": "OUT-025", "name": "Cirebon Kota"},
    "sales_division": {"id": "division_01", "code": "M1_BIS", "name": "M1 BIS"},
    "requester": {
      "user_id": "usr_laras",
      "name": "Laras P.",
      "role": {"id": "role_sa", "code": "SA", "name": "Sales Admin"}
    },
    "created_by": {"id": "usr_admin", "name": "Andi P."},
    "quantity": 6,
    "priority": "HIGH",
    "status": "IN_APPROVAL",
    "revision_no": 0,
    "version": 1,
    "approval": {
      "instance_id": "wfi_uuid",
      "status": "RUNNING",
      "steps": [
        {"sequence_no": 1, "role_code": "SS", "status": "PENDING"},
        {"sequence_no": 2, "role_code": "RSM", "status": "WAITING"},
        {"sequence_no": 3, "role_code": "GRSM", "status": "WAITING"},
        {"sequence_no": 4, "role_code": "NSM", "status": "WAITING"},
        {"sequence_no": 5, "role_code": "SD", "status": "WAITING"}
      ]
    },
    "submitted_at": "2026-07-12T02:00:00Z"
  }
}
```

### 8.8 Edit dan Resubmit Revision

Hanya request `REVISION_REQUIRED` yang dapat diedit.

```http
PATCH /api/v1/requests/{request_id}
```

```json
{
  "quantity": 4,
  "notes": "Quantity dikoreksi sesuai kebutuhan outlet",
  "version": 3
}
```

Response mengembalikan resource dengan version baru tetapi status tetap `REVISION_REQUIRED`.

```http
POST /api/v1/requests/{request_id}/resubmit
Idempotency-Key: <uuid>
```

```json
{
  "version": 4,
  "comment": "Data sudah diperbaiki"
}
```

Response: `200 OK`, status kembali `IN_APPROVAL` atau langsung `APPROVED` jika tidak ada task tersisa.

### 8.9 Cancel Request

```http
POST /api/v1/requests/{request_id}/cancel
```

```json
{
  "version": 2,
  "reason": "Kebutuhan dibatalkan"
}
```

Allowed dari `SUBMITTED`, `IN_APPROVAL`, atau `REVISION_REQUIRED` sebelum fulfillment dimulai. Approver task aktif dan waiting menjadi `CANCELLED`.

### 8.10 Approval Queue

```http
GET /api/v1/approval-tasks?scope=MY_TURN&status=PENDING&page=1&page_size=20
```

`scope`:

- `MY_TURN`: task yang dapat ditindak user;
- `HISTORY`: task yang pernah ditindak user;
- `ALL`: Admin/Auditor only.

List item mengikuti UI: request number, type, outlet, quantity, requester, step progress, current role, dan activated time.

### 8.11 Approval Detail

```http
GET /api/v1/approval-tasks/{task_id}
```

Response berisi approval task, request detail, seluruh chain, immutable approval history, dan `allowed_actions`.

```json
{
  "data": {
    "task": {
      "id": "task_uuid",
      "status": "PENDING",
      "version": 1,
      "role": {"code": "SS", "name": "Sales Supervisor"},
      "allowed_actions": ["APPROVE", "REQUEST_REVISION", "REJECT"]
    },
    "request": {"id": "req_uuid", "request_no": "REQ-2093", "quantity": 6},
    "chain": [],
    "history": []
  }
}
```

### 8.12 Approval Actions

Approve:

```http
POST /api/v1/approval-tasks/{task_id}/approve
Idempotency-Key: <uuid>
```

```json
{
  "version": 1,
  "comment": "Approved"
}
```

Request revision:

```http
POST /api/v1/approval-tasks/{task_id}/request-revision
```

```json
{
  "version": 1,
  "comment": "Mohon koreksi quantity dan outlet tujuan"
}
```

Reject:

```http
POST /api/v1/approval-tasks/{task_id}/reject
```

```json
{
  "version": 1,
  "comment": "Request tidak sesuai kebijakan"
}
```

Rules:

- Hanya task `PENDING` dapat ditindak.
- User harus memiliki role dan scope task tersebut.
- Comment optional untuk approve, wajib minimal 10 karakter untuk revision/reject.
- Action yang sama dengan idempotency key sama mengembalikan hasil pertama.
- Approve mengaktifkan task berikutnya; final approve membuat fulfillment.

### 8.13 Fulfillment Queue dan Detail

```http
GET /api/v1/fulfillments?status=PROCESSING&asset_type=ANDROID&page=1&page_size=20
```

```http
GET /api/v1/fulfillments/{fulfillment_id}
```

Detail mencakup request summary, expected quantity, identifier rule, PIC, items, progress, dan event history.

### 8.14 Start Fulfillment

```http
POST /api/v1/fulfillments/{fulfillment_id}/start
```

```json
{
  "version": 1,
  "assigned_to_user_id": "usr_asset_team",
  "notes": "Mulai persiapan perangkat"
}
```

Allowed: `PENDING -> PROCESSING`.

### 8.15 Add Fulfillment Item

```http
POST /api/v1/fulfillments/{fulfillment_id}/items
```

Android example:

```json
{
  "unit_no": 1,
  "identifier": {
    "type": "IMEI",
    "value": "352099001761481"
  },
  "asset_code": "AST-AN-000001",
  "notes": null
}
```

Server menggunakan `SERIAL_NUMBER`. Barcode boleh mengirim `identifier: null`. Identifier harus unique secara global.

Response: `201 Created` dengan fulfillment item dan asset yang diregistrasikan.

### 8.16 Fulfillment Transitions

Ship:

```http
POST /api/v1/fulfillments/{fulfillment_id}/ship
```

```json
{
  "version": 3,
  "tracking_no": "JNE-123456789",
  "notes": "Dikirim ke outlet"
}
```

Deliver/install:

```http
POST /api/v1/fulfillments/{fulfillment_id}/deliver
```

```json
{
  "version": 4,
  "installed": true,
  "received_by": "Budi - Outlet PIC",
  "notes": "Diterima dalam kondisi baik"
}
```

Complete:

```http
POST /api/v1/fulfillments/{fulfillment_id}/complete
```

```json
{
  "version": 5,
  "notes": "Seluruh unit selesai diproses"
}
```

Valid transition utama:

```text
PENDING -> PROCESSING -> SHIPPED -> DELIVERED -> COMPLETED
PENDING -> PROCESSING -> INSTALLED -> COMPLETED
```

Server dapat menggunakan flow installed tanpa shipping jika instalasi dilakukan langsung.

### 8.17 Asset Registry

```http
GET /api/v1/assets?asset_type=ANDROID&status=IN_USE&outlet_id=outlet_08&search=352099
```

Manual registration:

```http
POST /api/v1/assets
```

```json
{
  "asset_type_id": "asset_type_server",
  "asset_code": "AST-SR-000120",
  "distributor_id": "dist_01",
  "outlet_id": "outlet_21",
  "status": "REGISTERED",
  "identifiers": [
    {"type": "SERIAL_NUMBER", "value": "SRV-2026-000120", "is_primary": true}
  ]
}
```

### 8.18 Asset Assignment dan Return

Assign:

```http
POST /api/v1/assets/{asset_id}/assignments
```

```json
{
  "version": 2,
  "assigned_to_user_id": "usr_123",
  "outlet_id": "outlet_08",
  "notes": "Assigned sebagai device operasional"
}
```

Return:

```http
POST /api/v1/assets/{asset_id}/return
```

```json
{
  "version": 3,
  "condition": "GOOD",
  "returned_to_outlet_id": "outlet_08",
  "notes": "User pindah unit"
}
```

Assignment aktif sebelumnya ditutup dan asset history ditambahkan dalam satu transaction.

### 8.19 Master Data CRUD Pattern

List:

```http
GET /api/v1/outlets?active=true&search=Bandung&page=1&page_size=20
```

Create outlet:

```http
POST /api/v1/outlets
```

```json
{
  "code": "OUT-011",
  "name": "Bandung Kota",
  "region_id": "region_west_java",
  "is_active": true
}
```

Update:

```http
PATCH /api/v1/outlets/{outlet_id}
```

```json
{
  "name": "Bandung Kota",
  "region_id": "region_west_java",
  "is_active": true,
  "version": 2
}
```

Deactivate:

```http
DELETE /api/v1/outlets/{outlet_id}
```

Response: `204 No Content`. Delete adalah soft deactivate dan ditolak jika melanggar referential/business rule aktif.

Distributor outlet mapping:

```http
PUT /api/v1/distributors/{distributor_id}/outlets
```

```json
{
  "outlet_ids": ["outlet_bandung", "outlet_depok"],
  "version": 3
}
```

Asset type payload:

```json
{
  "code": "AN",
  "name": "Android",
  "identifier_type": "IMEI",
  "identifier_required": true,
  "is_active": true
}
```

### 8.20 Users dan Roles

Create user:

```http
POST /api/v1/users
```

```json
{
  "employee_no": "EMP00123",
  "name": "Laras P.",
  "email": "laras@company.co",
  "role_assignments": [
    {
      "role_id": "role_sales_supervisor",
      "distributor_id": "dist_01",
      "outlet_id": null,
      "is_primary": true
    }
  ]
}
```

Role codes minimum:

```text
ADMIN, ASSET_TEAM,
SA, SS, RSM, GRSM, NSM, SD,
CABANG_DISTRIBUTOR,
AUDITOR
```

### 8.21 Workflow Definition

Create draft workflow:

```http
POST /api/v1/workflow-definitions
```

```json
{
  "code": "BARCODE_APPROVAL",
  "name": "Barcode Approval",
  "source_system": "AMS",
  "reference_type": "ASSET_REQUEST",
  "asset_type_id": "asset_type_barcode",
  "steps": [
    {"sequence_no": 1, "role_id": "role_sa", "is_required": true},
    {"sequence_no": 2, "role_id": "role_ss", "is_required": true},
    {"sequence_no": 3, "role_id": "role_rsm", "is_required": true},
    {"sequence_no": 4, "role_id": "role_grsm", "is_required": true},
    {"sequence_no": 5, "role_id": "role_nsm", "is_required": true},
    {"sequence_no": 6, "role_id": "role_sd", "is_required": true}
  ]
}
```

Workflow endpoints:

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/workflow-definitions` | List definitions |
| POST | `/api/v1/workflow-definitions` | Create draft version |
| GET | `/api/v1/workflow-definitions/{id}` | Detail and steps |
| PATCH | `/api/v1/workflow-definitions/{id}` | Edit inactive draft |
| POST | `/api/v1/workflow-definitions/{id}/activate` | Activate version |
| POST | `/api/v1/workflow-definitions/{id}/retire` | Retire active version |

Activation rule: hanya satu active workflow binding untuk kombinasi source system, reference type, dan asset type pada waktu yang sama. Runtime instance selalu menyimpan definition version yang digunakan.

### 8.22 Notifications

```http
GET /api/v1/notifications?read=false&page=1&page_size=20
```

```json
{
  "data": [
    {
      "id": "notification_uuid",
      "type": "APPROVAL_REQUIRED",
      "title": "REQ-2093 menunggu approval",
      "message": "Barcode request dari Laras P. menunggu keputusan Anda.",
      "reference": {"type": "APPROVAL_TASK", "id": "task_uuid"},
      "read_at": null,
      "created_at": "2026-07-12T02:01:00Z"
    }
  ]
}
```

### 8.23 Audit Logs

```http
GET /api/v1/audit-logs?entity_type=ASSET_REQUEST&entity_id=req_uuid&action=UPDATE&page=1&page_size=50
```

Audit response mencakup actor, action, entity reference, timestamp, correlation request ID, dan before/after snapshot. Credential, access token, auth token, dan field sensitif tidak boleh dicatat.

## 9. State Transition Rules

### 9.1 Request

```text
SUBMITTED -> IN_APPROVAL
IN_APPROVAL -> REVISION_REQUIRED | APPROVED | REJECTED | CANCELLED
REVISION_REQUIRED -> IN_APPROVAL | CANCELLED
APPROVED -> IN_FULFILLMENT
IN_FULFILLMENT -> COMPLETED
```

### 9.2 Approval Task

```text
WAITING -> PENDING
PENDING -> APPROVED | REJECTED | REVISION_REQUESTED | CANCELLED
REVISION_REQUESTED -> PENDING after resubmit
```

### 9.3 Fulfillment

```text
PENDING -> PROCESSING | CANCELLED
PROCESSING -> SHIPPED | INSTALLED | DELIVERED | CANCELLED
SHIPPED -> DELIVERED
DELIVERED -> COMPLETED
INSTALLED -> COMPLETED
```

Invalid transition menghasilkan `409 INVALID_STATE_TRANSITION` dengan status sekarang dan allowed transitions.

## 10. Validation Rules

- `quantity`: integer 1-10000.
- `priority`: default `NORMAL`.
- `requester_name`: 2-150 karakter jika requester user tidak tersedia.
- `notes`: maksimum 1000 karakter.
- revision/reject comment: 10-1000 karakter.
- outlet harus aktif dan ter-map ke distributor.
- master data inactive tidak dapat dipakai untuk request baru.
- request type harus aktif dan dimiliki asset type terpilih.
- workflow active wajib tersedia sebelum request disubmit.
- hanya satu task `PENDING` per sequential workflow instance.
- IMEI dan serial number wajib unique secara global.
- jumlah fulfillment item tidak boleh melebihi request quantity.

## 11. Transaction dan Concurrency Boundaries

Operasi berikut wajib atomic:

- Create request + workflow instance + approval tasks + submit action.
- Final approval + request approved + fulfillment creation.
- Request revision/resubmit + workflow/task updates.
- Fulfillment item + asset + identifier registration.
- Asset assignment/return + asset status + history.

Semua command memeriksa resource `version`. Event notification dibuat dalam transaction/outbox agar tidak hilang walaupun pengiriman notification asynchronous.

## 12. Open Decisions Before Implementation

1. Corporate SSO/JWT issuer, claims, dan mapping employee identity.
2. Apakah role assignment di-scope berdasarkan distributor, outlet, region, atau kombinasi.
3. Daftar resmi Android request type.
4. Apakah Barcode menghasilkan asset record per unit atau hanya fulfillment output.
5. Apakah Server selalu melalui tahap installation.
6. Channel notification MVP: in-app saja atau ditambah email/Teams.
7. Retention period audit log dan attachment evidence.
8. Batas maksimum quantity per category.

Keputusan tersebut tidak mengubah struktur inti contract, tetapi harus dibekukan sebelum endpoint terkait diimplementasikan.
