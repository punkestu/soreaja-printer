# Soreaja Printer Microservice

A lightweight Go microservice that accepts image payloads over HTTP and sends them to local CUPS printers via the `lp` CLI command. Designed with dynamic printer selection, strict input sanitization, and defaults optimized for EPSON L120 series photo/document printing.

---

## 🛠 Features

- **Desktop GUI Control Panel:** Native desktop interface (Fyne v2) to start/stop the server in 1 click, view live access logs, and test color & thermal printer connections without terminal commands.
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
| `DEFAULT_COLOR_PRINTER_NAME` | Default color printer destination | `EPSON_L120_Series` |
| `DEFAULT_THERMAL_PRINTER_NAME`| Default thermal printer destination (fallback) | `/dev/cu.RPP02N` |
| `THERMAL_PRINTER_MAC` | Bluetooth MAC address for ESC/POS thermal printer | `""` |
| `MAX_UPLOAD_SIZE_BYTES` | Maximum allowed request payload size in bytes | `10485760` (10MB) |
| `PRINT_TIMEOUT_SECONDS` | Maximum duration before aborting print execution | `30` |

---

## 🚀 Getting Started

### Prerequisites

1. **Go 1.22+** installed.
2. For CUPS color printing: target printer queue configured (`lpstat -p -d`).
3. For thermal printing: Bluetooth thermal printer paired or device attached.

### Running the Application

```bash
# Clone and enter the repository
cd soreaja-printer

# Run tests
make test

# Build executable
make build

# Launch the Desktop GUI (Default)
make run

# Alternatively run directly via go:
go run .

# Run in headless CLI server mode (CI / Docker / daemon):
make run-headless
# or:
./soreaja-printer -headless -port 8080

# Clean build artifacts
make clean
```

---

## 🖥️ Desktop GUI Control Panel

The desktop interface provides an interactive workstation for printer operators:

1. **Server Management**:
   - **Start / Stop Toggle**: In-process HTTP microservice lifecycle control with visual status indicator (🟢 Running / 🔴 Stopped).
   - **Dynamic Port**: Configurable HTTP listening port (defaults to `8080`).
   - **Service Endpoints**: Copyable list of detected local and network LAN access URLs.
2. **Printer Diagnostics**:
   - **Color Printer (CUPS / Epson)**:
     - `Check CUPS Status`: Queries CUPS (`lpstat -p`) for printer state (idle, busy, offline).
     - `Print Test Swatch`: Generates and prints a full-color alignment and CMYK+RGB test pattern.
   - **Thermal Printer (ESC/POS / Bluetooth)**:
     - `Check Connection`: Tests reachability for Bluetooth MAC addresses, serial devices (`/dev/...`), or network sockets (`ip:port`).
     - `Print Test Receipt`: Generates and prints a diagnostic test receipt with banner, timestamp, density grid, feed, and paper cut.
3. **Live Logs Console**:
   - Real-time stream of incoming HTTP requests, response status codes, and service logs.
   - Auto-scroll and 1-click log clearing.

---

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
| `printer` | String | Target CUPS printer name or keyword (`thermal`, `color`, or specific queue name). Defaults to thermal printer if omitted. | No |

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
