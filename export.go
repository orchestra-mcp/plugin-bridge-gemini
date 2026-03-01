package bridgegemini

import (
	"github.com/orchestra-mcp/plugin-bridge-gemini/internal"
	"github.com/orchestra-mcp/sdk-go/plugin"
)

// Register adds all Gemini bridge tools to the builder.
func Register(builder *plugin.PluginBuilder) {
	bp := internal.NewBridgePlugin()
	bp.RegisterTools(builder)
}
