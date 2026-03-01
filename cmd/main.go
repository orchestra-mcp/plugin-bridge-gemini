// Command bridge-gemini is the entry point for the bridge.gemini plugin
// binary. It calls the Google Gemini API directly and manages multi-turn
// conversation sessions in memory.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/orchestra-mcp/sdk-go/plugin"
	"github.com/orchestra-mcp/plugin-bridge-gemini/internal"
)

func main() {
	builder := plugin.New("bridge.gemini").
		Version("0.1.0").
		Description("Gemini AI bridge plugin").
		Author("Orchestra").
		Binary("bridge-gemini").
		ProvidesAI("gemini")

	bp := internal.NewBridgePlugin()
	bp.RegisterTools(builder)

	p := builder.BuildWithTools()
	p.ParseFlags()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		bp.CloseAll() // mark all sessions as completed
		cancel()
	}()

	if err := p.Run(ctx); err != nil {
		log.Fatalf("bridge.gemini: %v", err)
	}
}
