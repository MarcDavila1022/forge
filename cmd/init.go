/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"text/template"

	"github.com/spf13/cobra"

	"os/exec"

	"bytes"
)

type WorkflowConfig struct {
	RepoName string
}

func getRepoName() string {
	remoteName, _ := exec.Command("git", "remote", "get-url", "origin").CombinedOutput()

	startIdx := bytes.LastIndex(remoteName, []byte("/")) + 1
	endIdx := bytes.LastIndex(remoteName, []byte("."))

	return string(remoteName[startIdx:endIdx])

}

var workflowTemplate = `name: Secure Release - {{.RepoName}}
on: 
  push:
  tags:
  - 'v*'
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		config := WorkflowConfig{
			RepoName: getRepoName(),
		}
		tmpl := template.Must(template.New("workflow").Parse(workflowTemplate))
		var buf bytes.Buffer
		tmpl.Execute(&buf, config)

		os.MkdirAll(".github/workflows", 0755)

		os.WriteFile(".github/workflows/secure-release.yml", buf.Bytes(), 0644)
		fmt.Println("Created .github/workflows/secure-release.yml")

	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// initCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
