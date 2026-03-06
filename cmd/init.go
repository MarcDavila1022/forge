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
	ScanGate bool
}

func getRepoName() string {
	remoteName, _ := exec.Command("git", "remote", "get-url", "origin").CombinedOutput()

	startIdx := bytes.LastIndex(remoteName, []byte("/")) + 1
	endIdx := bytes.LastIndex(remoteName, []byte("."))

	return string(remoteName[startIdx:endIdx])

}

var workflowTemplate = `name: Secure Release - [[.RepoName]]
on:
  push:
    tags:
      - 'v*'
permissions:
  contents: write
  id-token: write
  attestations: write
jobs:
  release:
    runs-on: ubuntu-latest
    steps: 
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
[[ if .ScanGate ]]
      - name: Install forge
        run: go install github.com/marcdavila/forge@latest
      - name: Vulnerability Gate
        run: forge scan --severity high
[[ end ]]
      - name: Build
        run: |
          mkdir -p dist
          GOOS=darwin GOARCH=arm64 go build -ldflags "-X 'github.com/marcdavila/forge/cmd.Version=${{ github.ref_name }}'" -o dist/forge-darwin-arm64 .
          GOOS=linux GOARCH=amd64 go build -ldflags "-X 'github.com/marcdavila/forge/cmd.Version=${{ github.ref_name }}'" -o dist/forge-linux-amd64 .
          GOOS=windows GOARCH=amd64 go build -ldflags "-X 'github.com/marcdavila/forge/cmd.Version=${{ github.ref_name }}'" -o dist/forge-windows-amd64.exe .
          GOOS=linux GOARCH=arm64 go build -ldflags "-X 'github.com/marcdavila/forge/cmd.Version=${{ github.ref_name }}'" -o dist/forge-linux-arm64 .
          GOOS=darwin GOARCH=amd64 go build -ldflags "-X 'github.com/marcdavila/forge/cmd.Version=${{ github.ref_name }}'" -o dist/forge-darwin-amd64 .
      - name: Generate checksums
        run: |
          cd dist
          sha256sum * > checksums.txt
      - uses: actions/attest-build-provenance@v2
        with:
          subject-path: dist/*  
      - name: Create Release
        run: |
          gh release create ${{ github.ref_name }} dist/* --generate-notes
        env: 
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
`
var scanGateFlag bool

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
			ScanGate: scanGateFlag,
		}
		tmpl := template.Must(template.New("workflow").Delims("[[", "]]").Parse(workflowTemplate))
		var buf bytes.Buffer
		tmpl.Execute(&buf, config)

		os.MkdirAll(".github/workflows", 0755)

		os.WriteFile(".github/workflows/secure-release.yml", buf.Bytes(), 0644)
		fmt.Println("Created .github/workflows/secure-release.yml")

	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVar(&scanGateFlag, "scan-gate", false, "Inject a vulnerability gate step into the workflow")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// initCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
