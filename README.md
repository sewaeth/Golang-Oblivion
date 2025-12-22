# Golang-Oblivion

A production-ready Discord bot written in Go, managing parallel and sequential webhook shard modes with comprehensive logging and security features.

## Setup

1. **Install Go** (1.21 or higher)
   ```bash
   # Visit: https://golang.org/doc/install
   ```

2. **Install Dependencies**
   ```bash
   go mod download
   ```

3. **Configure Bot**
   - Create Discord app at https://discord.com/developers/applications
   - Copy bot token
   - **Recommended:** Set environment variable:
     ```bash
     export OBLIVION_BOT_TOKEN="your-bot-token-here"
     ```
   - **Alternative:** Edit `config.toml`

4. **Configure Webhooks**
   - Edit `webhooks.toml`
   - Add your webhook URLs under shard names

5. **Run**
   ```bash
   make build
   make run
   ```

## Commands

- `/config [field] [value]` - View/modify configuration
- `/start <mode>` - Start operations (parallel/sequential)
- `/stop` - Stop all operations

## Development

```bash
make build     # Build binary
make test      # Run tests
make lint      # Format and vet
make coverage  # Coverage report
make clean     # Remove artifacts
```

## Configuration

See `.env.example` for environment variables.
See `config.toml` for all configuration options with validation.

## Security Features

- Environment variable secrets
- Input validation (1-2000 char messages, 1-10 retries, etc.)
- Audit logging for all commands
- .gitignore protection

## Logs

Structured JSON logs include contextual fields:
```json
{"level":"INFO","msg":"Command executed","command":"start","user_id":"123","guild_id":"456"}
```

Pipe through `jq` for pretty printing:
```bash
./bin/oblivion | jq
```

## Architecture

- `/config` - Safe configuration with validation
- `/start` - Parallel (multi-shard) or Sequential (rotation) modes  
- `/stop` - Graceful shutdown with sync.Once protection

## Troubleshooting

**Bot won't start:** Check `OBLIVION_BOT_TOKEN` is set  
**Commands missing:** Verify `applications.commands` scope  
**Logs not showing:** Use JSON parser like `jq`

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License
