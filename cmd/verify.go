/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os/exec"

	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type RequiredPolicy struct {
	Provenance bool `yaml:"provenance"`
}

type HardenConfig struct {
	Dir  string `yaml:"dir"`
	Skip string `yaml:"skip"`
}

type ForgeConfig struct {
	Repo    string         `yaml:"repo"`
	Require RequiredPolicy `yaml:"require"`
	Harden  HardenConfig   `yaml:"harden"`
}

// verifyCmd represents the verify command
var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		file := args[0]
		content, err := os.ReadFile("forge.yml")
		if err != nil {
			fmt.Println("Error: forge.yml not found")
			return
		}
		var config ForgeConfig
		yaml.Unmarshal(content, &config)

		if config.Repo == "" {
			fmt.Println("Error: repo not set in forge.yml")
			return
		}
		if config.Require.Provenance {
			_, attestationErr := exec.Command("gh", "attestation", "verify", file,
				"--repo", config.Repo, "--format", "json").CombinedOutput()
			if attestationErr != nil {
				fmt.Printf("\u274C NOT VERIFIED\n")
			} else {
				fmt.Printf("\u2705 VERIFIED")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// verifyCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// verifyCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
