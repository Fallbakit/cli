# Fallbakit CLI

`fallbakit` is the developer CLI for [Fallbakit](https://fallbakit.com) — local-first AI inference (Ollama, oMLX, vLLM) with automatic cloud fallback behind one OpenAI-compatible API.

## Install

```sh
# curl | sh
curl -fsSL https://fallbakit.com/install.sh | sh

# Homebrew
brew install fallbakit/homebrew-tap/fallbakit

# Go
go install github.com/fallbakit/cli/cmd/fallbakit-cli@latest
```

## Usage

```sh
fallbakit login            # device-flow auth against the dashboard
fallbakit runner create    # register a local runner
fallbakit runner up        # start the tunnel agent for a runner
fallbakit app init         # scaffold an app against the OpenAI-compatible API
fallbakit version
```

The CLI talks to the Fallbakit dashboard management API with a `cli_*` token. The local tunnel agent ships separately from [`fallbakit/tunnel`](https://github.com/fallbakit/tunnel).

## Development

```sh
go build ./...
go test ./...
go run ./cmd/fallbakit-cli version
```

## License

[Apache-2.0](LICENSE).
