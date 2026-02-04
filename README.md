# AI Limits Monitor

A Mattermost plugin for monitoring AI service usage and billing limits.

## Supported Services

| Service | Status | What's Monitored |
|---------|--------|------------------|
| **Augment Code** | ✅ Full | Credits used/remaining, plan, billing cycle |
| **Z.AI** | ✅ Full | Token quota (5h window), MCP tools, subscription |
| **OpenAI** | ✅ Full* | Organization costs (* requires API key with `api.usage.read` scope) |
| **Claude** | ⚠️ Partial | Requires admin API key for full usage data |

## Installation

1. Download the latest release from [Releases](https://github.com/alexander-voronkov/mattermost-ai-limits-monitor/releases)
2. Go to **System Console → Plugins → Plugin Management**
3. Upload the `.tar.gz` bundle
4. Enable the plugin
5. Configure API keys in **System Console → Plugins → AI Limits Monitor**

## Usage

Click the 📊 icon in the channel header (or AppBar in Mattermost 10+) to open the AI Limits panel.

The panel shows:
- Real-time usage bars for each configured service
- Color-coded status indicators (green → yellow → red)
- Auto-refresh every 5 minutes
- Manual refresh button for instant updates

## Building

### Prerequisites
- Go 1.22+
- Node.js 22+
- npm

### Build
```bash
make build    # Build all platforms + webapp + bundle
make clean    # Clean build artifacts
```

### Development
```bash
make server-local   # Build server for current platform only
cd webapp && npm run build:watch   # Watch mode for webapp
```

## Compatibility

- Mattermost 10.x - 11.x

## License

MIT
