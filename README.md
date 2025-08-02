# AlertManager to Google Chat Webhook Bridge

A **production-ready Golang application** that receives **Prometheus AlertManager** webhook notifications and forwards alerts to **Google Chat** with rich, formatted messages.

## **Features**
- ✅ **Production Ready**: Graceful shutdown, proper error handling, and comprehensive logging
- ✅ **Security**: HTTPS validation, non-root container user, input validation
- ✅ **Monitoring**: Prometheus metrics, health checks, structured logging
- ✅ **Reliability**: Retry logic, timeout handling, request validation
- ✅ **Maintainability**: Clean code structure, comprehensive tests, DRY principles

## **Prerequisites**
- **Go 1.24.1 or higher**
- A **Google Chat webhook URL**
- Configured **AlertManager** to send webhooks
- **Docker** (for containerized deployment)

## **Quick Start**

### Local Development
```bash
git clone <repository-url>
cd alertmanager-to-gchat
go mod tidy
go build -o alertmanager-to-gchat
./alertmanager-to-gchat --config ./config.toml
```

### Docker Deployment
```bash
docker build -t alertmanager-to-gchat .
docker run -p 7000:7000 \
  -e GOOGLE_CHAT_WEBHOOK_URL="https://chat.googleapis.com/v1/spaces/XXXXX/messages?key=YYYYY&token=ZZZZZ" \
  alertmanager-to-gchat
```

## **Configuration**

### Configuration File (`config.toml`)
```toml
[server]
listen_addr = ":7000"

[google_chat]
webhook_url = "https://chat.googleapis.com/v1/spaces/XXXXX/messages?key=YYYYY&token=ZZZZZ"

[logging]
level = "info"  # debug, info, error
```

### Environment Variables
All configuration can be overridden with environment variables:
```bash
export LISTEN_ADDR=":7000"
export GOOGLE_CHAT_WEBHOOK_URL="https://chat.googleapis.com/v1/spaces/XXXXX/messages?key=YYYYY&token=ZZZZZ"
export LOG_LEVEL="info"
```

### 🔒 **Security Note**
**Important**: Replace the placeholder webhook URL in `config.toml` with your actual Google Chat webhook URL. Never commit real webhook URLs to version control. Use environment variables in production.

## **AlertManager Configuration**

Configure **AlertManager** to send alerts to your application:

```yaml
global:
  resolve_timeout: 5m

route:
  group_by: ['alertname', 'instance']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  receiver: 'google-chat'

receivers:
- name: 'google-chat'
  webhook_configs:
  - url: 'http://your-server:7000/webhook'
    send_resolved: true
    http_config:
      timeout: 10s
```

## **Production Deployment**

### Docker Compose
```yaml
version: '3.8'
services:
  alertmanager-to-gchat:
    build: .
    ports:
      - "7000:7000"
    environment:
      - GOOGLE_CHAT_WEBHOOK_URL=${GOOGLE_CHAT_WEBHOOK_URL}
      - LOG_LEVEL=info
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:7000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

### Kubernetes Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alertmanager-to-gchat
spec:
  replicas: 2
  selector:
    matchLabels:
      app: alertmanager-to-gchat
  template:
    metadata:
      labels:
        app: alertmanager-to-gchat
    spec:
      containers:
      - name: alertmanager-to-gchat
        image: alertmanager-to-gchat:latest
        ports:
        - containerPort: 7000
        env:
        - name: GOOGLE_CHAT_WEBHOOK_URL
          valueFrom:
            secretKeyRef:
              name: gchat-webhook-secret
              key: webhook-url
        - name: LOG_LEVEL
          value: "info"
        livenessProbe:
          httpGet:
            path: /health
            port: 7000
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 7000
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "100m"
---
apiVersion: v1
kind: Service
metadata:
  name: alertmanager-to-gchat
spec:
  selector:
    app: alertmanager-to-gchat
  ports:
  - port: 7000
    targetPort: 7000
  type: ClusterIP
```

## **Monitoring & Observability**

### Health Check Endpoint
```bash
curl http://localhost:7000/health
```
Response:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "version": "1.0.0"
}
```

### Prometheus Metrics
Available at `http://localhost:7000/metrics`:
- `alertmanager_gchat_alerts_received_total` - Total alerts received
- `alertmanager_gchat_alerts_sent_total` - Total alerts sent to Google Chat
- `alertmanager_gchat_processing_duration_seconds` - Alert processing time
- `alertmanager_gchat_provider_request_duration_seconds` - Provider request time
- `alertmanager_gchat_provider_errors_total` - Provider errors

### Logging
Structured logging with different levels:
- `DEBUG`: Detailed request/response information
- `INFO`: General application events
- `ERROR`: Error conditions and failures

## **Security Considerations**

### Production Security Checklist
- ✅ **HTTPS Only**: Webhook URLs must use HTTPS
- ✅ **Non-root User**: Container runs as non-root user
- ✅ **Input Validation**: All inputs are validated
- ✅ **Rate Limiting**: Consider implementing rate limiting
- ✅ **Authentication**: Consider adding webhook authentication
- ✅ **Network Security**: Use firewalls and network policies

### Recommended Security Enhancements
```yaml
receivers:
- name: 'google-chat'
  webhook_configs:
  - url: 'http://your-server:7000/webhook'
    send_resolved: true
    http_config:
      authorization:
        type: Bearer
        credentials: "your-secret-token"
```

## **Testing**

### Run Tests
```bash
go test -v ./...
```

### Test Coverage
```bash
go test -cover ./...
```

### Integration Testing
```bash
./alertmanager-to-gchat --config ./config.toml

curl -X POST http://localhost:7000/webhook \
  -H "Content-Type: application/json" \
  -d @test_webhook/sample_alert.json
```

## **Troubleshooting**

### Common Issues

1. **Webhook URL Invalid**
   ```
   Error: Google Chat webhook URL must use HTTPS
   ```
   Solution: Ensure webhook URL starts with `https://`

2. **Connection Timeout**
   ```
   Error: error sending request: context deadline exceeded
   ```
   Solution: Check network connectivity and webhook URL validity

3. **Invalid JSON Payload**
   ```
   Error: Error parsing AlertManager payload
   ```
   Solution: Verify AlertManager is sending valid JSON

### Debug Mode
Enable debug logging for troubleshooting:
```bash
export LOG_LEVEL=debug
./alertmanager-to-gchat --config ./config.toml
```

## **Performance Considerations**

- **Concurrent Requests**: Handles multiple concurrent webhook requests
- **Memory Usage**: Minimal memory footprint (~10-20MB)
- **CPU Usage**: Low CPU usage for typical alert volumes
- **Network**: Optimized HTTP client with connection pooling

## **Contributing**

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

## **License**

This project is licensed under the MIT License - see the LICENSE file for details.

