package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rifat977/taplytix/internal/config"
)

const version = "v0.1.0"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:   "taplytix",
		Short: "Terminal-native developer monitoring dashboard",
	}
	root.PersistentFlags().StringVarP(&configPath, "config", "c", "taplytix.toml", "path to config file")

	startCmd := newStartCmd(&configPath)
	root.AddCommand(startCmd)
	root.AddCommand(newInitCmd())
	root.AddCommand(newVersionCmd())

	root.RunE = startCmd.RunE

	return root
}

func newStartCmd(configPath *string) *cobra.Command {
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
			fmt.Fprintf(cmd.OutOrStdout(), "starting taplytix — config loaded (%d source(s), refresh %dms)\n",
				len(cfg.Sources), cfg.Server.RefreshMs)
			return nil
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
