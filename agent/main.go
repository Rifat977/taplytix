// taplytix-agent: a tiny sidecar that runs near an application, accepts
// OTLP traffic locally, and forwards every event over WebSocket to one or
// more taplytix dashboards.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rifat977/taplytix/internal/agent"
	"github.com/rifat977/taplytix/internal/receiver"
)

const version = "v0.1.0"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var listen, otlpGRPC, otlpHTTP, certFile, keyFile string

	root := &cobra.Command{
		Use:   "taplytix-agent",
		Short: "Sidecar that forwards OTLP telemetry to a taplytix dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(listen, otlpGRPC, otlpHTTP, certFile, keyFile)
		},
	}
	root.Flags().StringVar(&listen, "listen", ":7777", "WebSocket listen address")
	root.Flags().StringVar(&otlpGRPC, "otlp-grpc", ":4317", "OTLP gRPC listen address")
	root.Flags().StringVar(&otlpHTTP, "otlp-http", ":4318", "OTLP HTTP listen address")
	root.Flags().StringVar(&certFile, "cert", "", "TLS certificate (optional)")
	root.Flags().StringVar(&keyFile, "key", "", "TLS key (optional)")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "taplytix-agent %s\n", version)
		},
	})
	return root
}

func run(listen, otlpGRPC, otlpHTTP, certFile, keyFile string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	out := make(chan any, 4096)
	otlp := receiver.NewOTLP("agent-otlp", otlpGRPC, otlpHTTP)
	if err := otlp.Start(ctx, out); err != nil {
		return fmt.Errorf("start OTLP receiver: %w", err)
	}
	defer otlp.Stop()

	bcast := agent.NewBroadcaster()
	go bcast.Pump(ctx, out)

	fmt.Fprintf(os.Stderr, "taplytix-agent listening: WS=%s OTLP-gRPC=%s OTLP-HTTP=%s\n",
		listen, otlpGRPC, otlpHTTP)

	if certFile != "" && keyFile != "" {
		return bcast.ListenAndServeTLS(ctx, listen, certFile, keyFile)
	}
	return bcast.ListenAndServe(ctx, listen)
}
