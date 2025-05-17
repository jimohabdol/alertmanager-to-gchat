# Alertmanager to Google Chat Webhook Bridge

A **Golang application** that receives **Prometheus Alertmanager** webhook notifications and forwards alerts to **Google Chat** with rich, formatted messages.

## **Features**
- Listens for **Alertmanager webhook** notifications.
- Converts alerts into **Google Chat message cards**.
- Includes labels, annotations, and links to **Prometheus & Alertmanager**.

## **Prerequisites**
- **Go 1.16 or higher**
- A **Google Chat webhook URL**
- Configured **Alertmanager** to send webhooks.

## **Installation**
Clone the repository and build the application:
```bash
git clone seamfix/alertmanger-to-gchat
cd alertmanager-to-gchat
go build -o alertmanager-to-gchat
```

## **Usage**
Run the application:
```bash
./alertmanager-to-gchat --config ./config.toml
```

## **Alertmanager Configuration**
Modify **Alertmanager** to send alerts to your application:
```yaml
receivers:
- name: 'google-chat'
  webhook_configs:
  - url: 'http://localhost:7000/webhook'
    send_resolved: true

route:
  receiver: 'google-chat'
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  group_by: ['alertname', 'instance']
```

## **Docker Support**
Build and run using **Docker**:
```bash
docker build -t alertmanager-to-gchat .
docker run -p 7000:7000 alertmanager-to-gchat

## **Google Chat Message**
The application generates **formatted message cards** in Google Chat with:
- **Alert name & status**.
- **Summary with labels & annotations**.
- **Direct links to Prometheus & Alertmanager**.
- **Color-coded alert severity**.

