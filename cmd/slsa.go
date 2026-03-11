package cmd

import slsacmd "github.com/marcdavila/forge/cmd/slsa"

func init() {
	rootCmd.AddCommand(slsacmd.NewCommand(Version))
}
