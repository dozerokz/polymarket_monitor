# Polymarket Monitor

A lightweight Go application that monitors Polymarket wallet activity and sends real-time notifications to Telegram when trades are detected.

![Polymarket monitor](https://github-production-user-asset-6210df.s3.amazonaws.com/60073740/525572030-0bba8d35-a512-49a1-a5f7-d106bebf683f.png?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAVCODYLSA53PQK4ZA%2F20251211%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20251211T195945Z&X-Amz-Expires=300&X-Amz-Signature=9ef67701a81f11a6a11aa7595ce2e4b304df74744a0e557440af4e2ecea413c4&X-Amz-SignedHeaders=host)


## Features

- Monitors multiple Polymarket wallet addresses simultaneously

- Tracks buy and sell trades in real-time

- Sends formatted notifications to Telegram

## Prerequisites

- Go 1.19 or higher

- Telegram Bot Token

- Telegram Chat ID

## Installation

1. Clone the repository:

```bash
git clone https://github.com/dozerokz/polymarket_monitor.git
cd polymarket_monitor
```

2. Install dependencies:

```bash
go mod download
```
3. Create a `.env` file in the root directory:

```env
TG_BOT_TOKEN=your_telegram_bot_token_here
CHAT_ID=your_telegram_chat_id_here
```

4. Paste POLYMARKET wallet addresses into `wallets.txt` file in the root directory(one per line)

## Configuration

The .env file requires the following variables:


- `TG_BOT_TOKEN` - Your Telegram bot token (obtain from @BotFather)

- `CHAT_ID` - The Telegram chat ID where notifications will be sent

The `.env` file is not included in the repository for security reasons.

## Usage

Run the application:

```bash
go run cmd/monitor/main.go
```

Or build and run the executable:

```bash
go build -o polymarket_monitor cmd/monitor/main.go
./polymarket_monitor
```

## Project Structure
```
polymarket_monitor/
├── cmd/
│   └── monitor/
│       └── main.go
├── internal/
│   ├── files_readers/
│   │   └── files_readers.go
│   ├── notifier/
│   │   └── telegram.go
│   └── parser/
│       ├── parser.go
│       └── model.go
├── .env (create this)
├── wallets.txt (edit this)
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

## How It Works

- Reads wallet addresses from wallets.txt

- Initializes monitoring by fetching recent activity for each wallet

- Continuously polls the Polymarket API for new activity

- Compares new activity against cached data

- Sends Telegram notifications for detected trades

- Updates cache with latest activity

## License

This project is open-source. You can use, modify, and distribute it under the [MIT License](LICENSE).
## Contributing

Contributions are welcome. Please open an issue or submit a pull request.