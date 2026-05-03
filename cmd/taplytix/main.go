package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/rifat977/taplytix/internal/bus"
	"github.com/rifat977/taplytix/internal/config"
	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/receiver"
	"github.com/rifat977/taplytix/internal/store"
	"github.com/rifat977/taplytix/internal/tui"
	"github.com/rifat977/taplytix/internal/tui/panels"
)

const version = "v0.1.0"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configPath string
	var logsPath string

	root := &cobra.Command{
		Use:   "taplytix",
		Short: "Terminal-native developer monitoring dashboard",
	}
	root.PersistentFlags().StringVarP(&configPath, "config", "c", "taplytix.toml", "path to config file")
	root.PersistentFlags().StringVar(&logsPath, "logs", "", "tail log file (overrides config; use '-' for stdin)")

	startCmd := newStartCmd(&configPath, &logsPath)
	root.AddCommand(startCmd)
	root.AddCommand(newInitCmd())
	root.AddCommand(newVersionCmd())

	root.RunE = startCmd.RunE

	return root
}

func newStartCmd(configPath, logsPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the taplytix dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				if !os.IsNotExist(unwrapPathErr(err)) {
					return err
				}
				cfg = config.Default()
				fmt.Fprintf(cmd.OutOrStderr(), "no config at %s — using defaults\n", *configPath)
			}
			if *logsPath != "" {
				cfg.Sources = append(cfg.Sources, config.SourceConfig{
					Name:   "logs",
					Type:   "logs",
					Path:   *logsPath,
					Format: "auto",
				})
			}
			return runTUI(cfg)
		},
	}
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Run the interactive setup wizard",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "setup wizard coming in phase 4")
			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "taplytix %s\n", version)
			return nil
		},
	}
}

func runTUI(cfg *config.Config) error {
	st := store.New()
	b := bus.New()

	ps := []panels.Panel{
		panels.NewOverview(cfg, st),
		panels.NewTraces(st),
		panels.NewMetrics(st),
		panels.NewLogs(st),
		panels.NewPlaceholder("Services"),
	}

	app := tui.NewApp(cfg, st, b, ps)
	prog := tea.NewProgram(app, tea.WithAltScreen())
	b.SetProgram(prog)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	receivers := startReceivers(ctx, cfg, st, b)
	defer func() {
		for _, r := range receivers {
			_ = r.Stop()
		}
	}()

	_, err := prog.Run()
	return err
}

// startReceivers spins up one receiver per configured source and pumps its
// events into the store + bus. Unsupported source types are skipped quietly
// so the TUI can still launch with a partial config.
func startReceivers(ctx context.Context, cfg *config.Config, st *store.Store, b *bus.Bus) []receiver.Receiver {
	out := make(chan any, 4096)
	go func() {
		for ev := range out {
			b.Publish(ev)
			switch e := ev.(type) {
			case model.MetricEvent:
				st.PushMetric(e)
			case model.SpanEvent:
				st.PushSpan(e)
			case model.LogEvent:
				st.PushLog(e)
			}
		}
	}()

	var started []receiver.Receiver
	for _, src := range cfg.Sources {
		var r receiver.Receiver
		switch src.Type {
		case "otlp":
			r = receiver.NewOTLP(src.Name, src.GRPC, src.HTTP)
		case "logs":
			r = receiver.NewLogTail(src.Name, receiver.LogTailOptions{
				Path:     src.Path,
				UseStdin: src.Path == "-",
				Format:   src.Format,
				Service:  src.Name,
			})
		default:
			continue
		}
		if err := r.Start(ctx, out); err != nil {
			fmt.Fprintf(os.Stderr, "receiver %s (%s) failed to start: %v\n", src.Name, src.Type, err)
			continue
		}
		started = append(started, r)
	}
	return started
}

func unwrapPathErr(err error) error {
	type unwrapper interface{ Unwrap() error }
	for err != nil {
		if pe, ok := err.(*os.PathError); ok {
			return pe
		}
		u, ok := err.(unwrapper)
		if !ok {
			return err
		}
		err = u.Unwrap()
	}
	return err
}
