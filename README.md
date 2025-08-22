# Go Uploader

A simple, secure file upload service written in Go with Cloudflare Turnstile CAPTCHA protection and support for both local and S3 storage backends.

## Features

- 🔒 **CAPTCHA Protection**: Cloudflare Turnstile integration to prevent spam uploads
- 📁 **Multiple Storage Backends**: Support for local filesystem and AWS S3 storage
- 🌐 **Web Interface**: Clean, responsive upload interface
- 🚀 **Lightweight**: Minimal dependencies and embedded static files
- 🐳 **Docker Support**: Ready-to-deploy container image
- 💚 **Health Checks**: Built-in health endpoint for monitoring
- 🔄 **Upload Resilience**: Automatic retry logic with exponential backoff for failed uploads
- ⏱️ **Timeout Handling**: Configurable timeouts to prevent hanging uploads
- 📊 **Progress Feedback**: Real-time upload status and progress indication
- 🛡️ **Error Recovery**: Smart error handling for connection issues and partial uploads

## Quick Start

### Prerequisites

- Go 1.24.3 or later
- Cloudflare Turnstile site key and secret
- (Optional) AWS account and S3 bucket for S3 storage

### Running with Go

1. Clone the repository:
```bash
git clone https://github.com/dseif0x/go-uploader.git
cd go-uploader
```

2. Set up environment variables (see [Environment Variables](#environment-variables)):
```bash
export TURNSTILE_SECRET="your-turnstile-secret"
# Add other variables as needed
```

3. Run the application:
```bash
go run main.go
```

The server will start on port 8080. Visit `http://localhost:8080` to access the upload interface.

### Running with Docker

1. Build the image:
```bash
docker build -t go-uploader .
```

2. Run the container:
```bash
docker run -p 8080:8080 \
  -e TURNSTILE_SECRET="your-turnstile-secret" \
  -e TURNSTILE_SITEKEY="your-turnstile-sitekey" \
  -v /path/to/uploads:/uploads \
  go-uploader
```

## Environment Variables

### Required Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `TURNSTILE_SECRET` | Cloudflare Turnstile secret key for CAPTCHA verification | `0x4AAAAAAABnH...` |
| `TURNSTILE_SITEKEY` | Cloudflare Turnstile site key for the frontend | `0x4AAAAAAABnH...` |

### Storage Backend Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `BACKEND` | Storage backend type (`local` or `s3`) | `local` | `s3` |

#### Local Storage Backend (BACKEND=local)

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `LOCAL_PATH` | Directory path for storing uploaded files | `./uploads` | `/var/uploads` |

#### S3 Storage Backend (BACKEND=s3)

When using S3 backend, the application uses AWS SDK v2 which supports multiple authentication methods:

**Option 1: Environment Variables**
| Variable | Description | Example |
|----------|-------------|---------|
| `AWS_ACCESS_KEY_ID` | AWS access key ID | `AKIAIOSFODNN7EXAMPLE` |
| `AWS_SECRET_ACCESS_KEY` | AWS secret access key | `wJalrXUtnFEMI/K7MDENG...` |
| `AWS_REGION` | AWS region where your S3 bucket is located | `us-east-1` |
| `AWS_ENDPOINT_URL` | AWS Endpoint URL for S3 compatible services (e.g. MinIO) | `https://play.min.io:9000` |

**Option 2: AWS Credentials File**
Place credentials in `~/.aws/credentials`:
```ini
[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

**Option 3: IAM Roles**
When running on AWS infrastructure, IAM roles can be used for authentication.

### Configuration with .env File

You can create a `.env` file in the project root to set environment variables:

```env
TURNSTILE_SECRET=your-turnstile-secret
BACKEND=local
LOCAL_PATH=./uploads

# For S3 backend:
# BACKEND=s3
# AWS_ACCESS_KEY_ID=your-access-key
# AWS_SECRET_ACCESS_KEY=your-secret-key
# AWS_REGION=us-east-1
```

## API Endpoints

### Upload Files
- **URL**: `/upload`
- **Method**: `POST`
- **Content-Type**: `multipart/form-data`
- **Headers**: 
  - `X-Turnstile-Token`: Cloudflare Turnstile token
- **Body**: Form data with file field(s)
- **Response**: `201 Created` with upload confirmation message

### Health Check
- **URL**: `/healthz`
- **Method**: `GET`
- **Response**: `200 OK` with "OK" body

### Static Files
- **URL**: `/`
- **Method**: `GET`
- **Description**: Serves the upload interface and static assets

## Upload Resilience Features

This application includes several features to make uploads more resilient to network connectivity issues:

### Backend Resilience
- **Server Timeouts**: Configurable read, write, and idle timeouts prevent hanging connections
- **Context-Based Cancellation**: Upload operations respect client disconnections and timeouts
- **Smart Error Handling**: Distinguishes between recoverable connection issues and permanent errors
- **Partial Success Support**: Tracks successful and failed file uploads separately
- **Enhanced Logging**: Detailed logging for debugging connectivity issues

### Frontend Resilience
- **Automatic Retry**: Failed uploads are automatically retried up to 3 times
- **Exponential Backoff**: Retry delays increase progressively (1s, 2s, 4s) to avoid overwhelming servers
- **Timeout Protection**: 5-minute upload timeout prevents indefinite hanging
- **Progress Indication**: Real-time status updates show upload progress and file counts
- **Error Classification**: Different error types receive appropriate handling (retry vs. fail)

### Error Handling
- **Connection Issues**: Automatic retry for timeout and EOF errors
- **Partial Uploads**: Clear indication when some files succeed and others fail
- **User Feedback**: Descriptive error messages help users understand and resolve issues

## Setup Instructions

### 1. Cloudflare Turnstile Setup

1. Go to [Cloudflare Dashboard](https://dash.cloudflare.com/)
2. Navigate to "Turnstile" section
3. Create a new site
4. Copy the site key and secret key
5. Set the site key in the `TURNSTILE_SITEKEY` environment variable
6. Set the secret key in the `TURNSTILE_SECRET` environment variable

### 2. Local Storage Setup

For local storage, ensure the upload directory exists and has proper permissions:

```bash
mkdir -p /path/to/uploads
chmod 755 /path/to/uploads
```

### 3. S3 Storage Setup

1. Create an S3 bucket in your AWS account
2. Set up IAM user with permissions to read/write to the bucket
3. Configure AWS credentials (see environment variables above)
4. The application will automatically create the `uploads/` prefix in your bucket

## Development

### Building

```bash
go build -o go-uploader
```

### Running Tests

```bash
go test ./...
```

### Linting

```bash
go vet ./...
go fmt ./...
```

## File Storage Details

### Local Storage
- Files are stored in the configured directory
- Each file is prefixed with a timestamp to avoid collisions
- Directory structure: `{LOCAL_PATH}/{timestamp}_{original_filename}`

### S3 Storage
- Files are stored in the configured S3 bucket
- Bucket name is hardcoded as `go-upload` in the source
- Files are stored under the `uploads/` prefix
- Object key format: `uploads/{timestamp}_{original_filename}`

## Security Considerations

- All uploads are protected by Cloudflare Turnstile CAPTCHA
- File names are sanitized to remove path traversal characters
- No file type restrictions are enforced by default
- Consider implementing file size limits for production use
- Ensure proper AWS IAM permissions when using S3 backend

## Docker Deployment

The included Dockerfile creates a minimal Alpine-based image:

```dockerfile
FROM alpine:latest
WORKDIR /go/bin
COPY --from=builder /go/bin/go-uploader /go/bin/go-uploader
ENTRYPOINT ["/go/bin/go-uploader"]
```

For production deployment, consider:
- Setting resource limits
- Configuring proper logging
- Using secrets management for environment variables
- Setting up load balancing for multiple instances

## License

This project is open source. Please check the repository for license details.
