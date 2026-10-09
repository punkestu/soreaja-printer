# Soreaja Printer Microservice

A lightweight Go microservice that accepts image payloads over HTTP and sends them to local CUPS printers via the `lp` CLI command. Designed with dynamic printer selection, strict input sanitization, and defaults optimized for EPSON L120 series photo/document printing.

---

## 🛠 Features

- **HTTP API:** Simple `multipart/form-data` endpoint `/api/v1/print`.
- **Image Validation:** Inspects magic bytes (JPEG & PNG supported) and enforces payload size limits.
- **Security:** Sanitizes printer names against command/argument injection (`^[a-zA-Z0-9_-]+$`).
- **CUPS Integration:** Maps directly to `lp` with print quality, resolution, and page sizing flags.
- **Graceful Lifecycle:** Handles `SIGINT` / `SIGTERM` signals and ensures disk cleanup of temporary print files.
- **Zero External Dependencies:** Built using Go standard library packages.

---

## ⚙️ Configuration

The microservice can be configured using environment variables:

| Environment Variable | Description | Default |
| :--- | :--- | :--- |
| `SERVER_PORT` | HTTP server listening port | `8080` |
| `DEFAULT_PRINTER_NAME` | Default printer destination if omitted in request | `EPSON_L120_Series` |
| `MAX_UPLOAD_SIZE_BYTES` | Maximum allowed request payload size in bytes | `10485760` (10MB) |
| `PRINT_TIMEOUT_SECONDS` | Maximum duration before aborting an `lp` execution | `30` |

---

## 🚀 Getting Started

### Prerequisites

1. **Go 1.22+** installed.
2. **CUPS** running with your target printer queue configured:
   ```bash
   lpstat -p -d
   ```
3. Permissions: The user running the binary must have permissions to submit print jobs (e.g. member of `lp` or `lpadmin` group).

### Running Locally

```bash
# Clone and enter the repository
cd soreaja-printer

# Run tests
go test -v ./...

# Build the executable
go build -o soreaja-printer .

# Start the server
./soreaja-printer
```

---

## 📡 API Reference

### 1. Print Image
* **URL:** `/api/v1/print`
* **Method:** `POST`
* **Content-Type:** `multipart/form-data`

#### Parameters:
| Field | Type | Description | Required |
| :--- | :--- | :--- | :--- |
| `image` | Binary File | Image file (`image/jpeg` or `image/png`) | **Yes** |
| `printer` | String | Target CUPS printer name (alphanumeric, `_`, `-`) | No (defaults to `DEFAULT_PRINTER_NAME`) |

#### Example Request:
```bash
curl -X POST http://localhost:8080/api/v1/print \
  -F "image=@/path/to/photo.jpg" \
  -F "printer=EPSON_L120_Series"
```

#### Success Response (`200 OK`):
```json
{
  "status": "success",
  "message": "Job sent to printer successfully",
  "job_id": "EPSON_L120_Series-123"
}
```

#### Error Response (`400 Bad Request` / `500 Internal Server Error`):
```json
{
  "status": "error",
  "message": "Invalid file format: only JPEG and PNG images are supported"
}
```

---

### 2. Health Check
* **URL:** `/health` or `/api/v1/health`
* **Method:** `GET`

#### Example Request:
```bash
curl http://localhost:8080/health
```

#### Success Response (`200 OK`):
```json
{
  "status": "ok",
  "message": "soreaja-printer service is healthy"
}
```
