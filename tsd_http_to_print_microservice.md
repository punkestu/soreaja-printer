# Technical Specification Document (TSD): HTTP-to-Print Microservice

## 1. Overview
This document outlines the architecture and implementation details for a lightweight Go (Golang) microservice. The application exposes an HTTP endpoint to accept image payloads and interfaces directly with the local CUPS print system. It prints the image using the `lp` command with specific formatting flags, allowing dynamic printer selection while defaulting to an EPSON L120 Series printer.

## 2. System Architecture
1. **Client Interface:** An external client sends a `multipart/form-data` HTTP POST request containing an image file and an optional target printer name.
2. **Go HTTP Server:** Parses the incoming payload, validates the file type, and securely writes the image to a temporary file on the host OS.
3. **Print Execution Engine:** Uses Go's `os/exec` package to spawn an `lp` shell process, passing the target printer name, required EPSON formatting flags, and the temporary file path.
4. **Cleanup & Response:** Deletes the temporary file from the disk and returns a standardized JSON HTTP response based on the `lp` exit status.

## 3. API Specification
### Endpoint: Print Image
*   **URL:** `/api/v1/print`
*   **Method:** `POST`
*   **Content-Type:** `multipart/form-data`

**Request Payload:**

| Field Name | Type          | Description                                                                 | Required |
| :--------- | :------------ | :-------------------------------------------------------------------------- | :------- |
| `image`    | File (Binary) | The image file to be printed (JPEG/PNG).                                    | Yes      |
| `printer`  | String        | Target CUPS printer destination. Defaults to `EPSON_L120_Series` if omitted.| No       |

**Success Response (200 OK):**
```json
{
  "status": "success",
  "message": "Job sent to printer successfully",
  "job_id": "EPSON_L120_Series-123"
}
```

**Error Response (400/500 Bad Request / Internal Server Error):**
```json
{
  "status": "error",
  "message": "Invalid file format or missing required fields" 
}
```

## 4. Implementation Details
### 4.1. Core Libraries
*   `net/http`: For serving the REST API.
*   `os/exec`: For executing the `lp` CLI command.
*   `os` & `io`: For temporary file creation and payload streaming.
*   `path/filepath`: For secure file path handling.
*   `regexp`: For sanitizing user inputs (printer name).

### 4.2. Command Execution Mapping
The application extracts the `printer` field from the multipart form. If the value is empty, it falls back to the configured default printer. The `lp` command maps to the following Go logic:

```go
// 1. Determine printer destination
printerName := r.FormValue("printer")
if printerName == "" {
    // Fallback to default from environment/config
    printerName = os.Getenv("DEFAULT_PRINTER_NAME")
    if printerName == "" {
        printerName = "EPSON_L120_Series"
    }
}

// 2. Construct command arguments
args := []string{
    "-d", printerName,
    "-o", "PageSize=Custom.288x432",
    "-o", "Resolution=720x720dpi",
    "-o", "EPIJ_Qual=306",
    "-o", "EPIJ_CMat=3",
    "-o", "EPIJ_AFil=1",
    "-o", "fit-to-page",
    tempFilePath,
}

// 3. Execute command with context (for timeouts)
cmd := exec.CommandContext(ctx, "lp", args...)
```

### 4.3. Configuration Management
To ensure reusability across environments, hardcoded values should be extracted into environment variables:
*   `SERVER_PORT`: Port to listen on (Default: `8080`)
*   `DEFAULT_PRINTER_NAME`: The fallback printer if the client omits it (Default: `EPSON_L120_Series`)
*   `MAX_UPLOAD_SIZE`: Payload limit to prevent memory exhaustion (Default: `10MB`)

## 5. Security & Lifecycle Considerations
*   **Printer Name Sanitization:** Because the `printer` parameter is user-supplied and passed directly as a command-line argument (`-d`), it must be strictly validated to prevent command argument injection. The app must validate that the `printer` string only contains alphanumeric characters, dashes, and underscores (e.g., using regex `^[a-zA-Z0-9_-]+$`).
*   **Input Validation:** The app must inspect the `Content-Type` header and sniff the file bytes using `http.DetectContentType` to ensure only `image/jpeg` or `image/png` are processed.
*   **File System Cleanup:** Temporary files must be wrapped in a `defer os.Remove(tempFile.Name())` statement immediately after creation to prevent disk space leaks in case of a panic or execution failure.
*   **Command Injection Prevention:** The `exec.Command` structure natively prevents shell injection by passing arguments directly to the binary rather than parsing them through `/bin/sh`.
*   **Execution Timeouts:** The `lp` command must be wrapped in a context with a timeout (e.g., `context.WithTimeout(context.Background(), 30*time.Second)`) to prevent the HTTP handler from hanging indefinitely if the CUPS daemon is unresponsive.

## 6. Infrastructure Requirements
*   **OS:** Linux/macOS host.
*   **Dependencies:** CUPS (`Common UNIX Printing System`) must be installed, running, and configured with the target printer queues (e.g., `EPSON_L120_Series`).
*   **Permissions:** The system user executing the Go binary must belong to the `lp` or `lpadmin` system group to authorize and submit print jobs successfully.