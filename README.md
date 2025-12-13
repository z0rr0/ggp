# GGP - Golden Gym Predictor Telegram Bot

![Go](https://github.com/z0rr0/ggp/workflows/Go/badge.svg)
![Version](https://img.shields.io/github/tag/z0rr0/ggp.svg)
![License](https://img.shields.io/github/license/z0rr0/ggp.svg)

A Telegram bot that tracks gym occupancy, predicts future load using statistical analysis, and displays visual charts.

## Features

- Periodic gym load data fetching from external API
- Load prediction using weighted statistical analysis with holiday awareness
- Visual charts for half-day, day, and week periods
- Holiday calendar integration
- CSV data import support
- Admin-only features via configuration

![schema](docs/image.png)

## Requirements

- Go 1.25 or later
- Telegram Bot API token (obtain from [@BotFather](https://t.me/botfather))
- Gym load data API endpoint
- Holiday calendar API endpoint (optional)

## Installation

```bash
make build
```

Or manually:

```bash
go build -o ggp .
```

## Configuration

Create a configuration file based on the example [config.example.toml](config.example.toml)

```bash
cp config.example.toml config.toml
```

Edit `config.toml` with your settings.

## Usage

```bash
./ggp -config config.toml
```

Import historical data from CSV:

```bash
./ggp -import data.csv -config config.toml
```

## Development

```bash
make test       # Run tests with race detector
make lint       # Run all linters
```

## Docker

```bash
make docker
docker-compose up -d
```

## Dependencies

- [go-telegram/bot](https://github.com/go-telegram/bot) - Telegram Bot API
- [pelletier/go-toml/v2](https://github.com/pelletier/go-toml) - TOML configuration
- [modernc.org/sqlite](https://modernc.org/sqlite) - SQLite driver (pure Go)
- [jmoiron/sqlx](https://github.com/jmoiron/sqlx) - Extensions to database/sql
- [go-chart/v2](https://github.com/wcharczuk/go-chart) - Chart generation

## License

MIT License - see [LICENSE](LICENSE) file for details.
