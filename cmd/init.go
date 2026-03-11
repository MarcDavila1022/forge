/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

type WorkflowConfig struct {
	RepoName string
	ScanGate bool
	L3       bool
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
var l3CallerTemplate = `name: Secure Release - [[.RepoName]]
on:
  push:
    tags:
      - 'v*'
jobs:
  release:
    uses: ./.github/workflows/build-and-attest.yml
    with:
      version: ${{ github.ref_name }}
`

var l3ReusableTemplate = `name: Build and Attest (Reusable)
on:
  workflow_call:
    inputs:
      version:
        required: true
        type: string
permissions:
  contents: write
  id-token: write
  attestations: write
jobs:
  build:
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
          GOOS=darwin GOARCH=arm64 go build -ldflags "-X 'github.com/marcdavila/forge/cmd.Version=${{ inputs.version }}'" -o dist/forge-darwin-arm64 .
          GOOS=linux GOARCH=amd64 go build -ldflags "-X 'github.com/marcdavila/forge/cmd.Version=${{ inputs.version }}'" -o dist/forge-linux-amd64 .
          GOOS=windows GOARCH=amd64 go build -ldflags "-X 'github.com/marcdavila/forge/cmd.Version=${{ inputs.version }}'" -o dist/forge-windows-amd64.exe .
          GOOS=linux GOARCH=arm64 go build -ldflags "-X 'github.com/marcdavila/forge/cmd.Version=${{ inputs.version }}'" -o dist/forge-linux-arm64 .
          GOOS=darwin GOARCH=amd64 go build -ldflags "-X 'github.com/marcdavila/forge/cmd.Version=${{ inputs.version }}'" -o dist/forge-darwin-amd64 .
      - name: Generate checksums
        run: |
          cd dist
          sha256sum * > checksums.txt
      - uses: actions/attest-build-provenance@v2
        with:
          subject-path: dist/*
      - name: Create Release
        run: gh release create ${{ inputs.version }} dist/* --generate-notes
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
`

var scanGateFlag bool
var l3Flag bool

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
			L3:       l3Flag,
		}

		os.MkdirAll(".github/workflows", 0755)

		// Check for existing files and confirm overwrite
		filesToWrite := []string{".github/workflows/secure-release.yml"}
		if l3Flag {
			filesToWrite = append(filesToWrite, ".github/workflows/build-and-attest.yml")
		}
		var existing []string
		for _, f := range filesToWrite {
			if _, err := os.Stat(f); err == nil {
				existing = append(existing, f)
			}
		}
		if len(existing) > 0 {
			fmt.Printf("The following files already exist and will be overwritten:\n")
			for _, f := range existing {
				fmt.Printf("  - %s\n", f)
			}
			fmt.Print("Continue? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Println("Aborted.")
				return
			}
		}

		if l3Flag {
			// Generate the reusable workflow (build + attest)
			reusableTmpl := template.Must(template.New("reusable").Delims("[[", "]]").Parse(l3ReusableTemplate))
			var reusableBuf bytes.Buffer
			reusableTmpl.Execute(&reusableBuf, config)
			os.WriteFile(".github/workflows/build-and-attest.yml", reusableBuf.Bytes(), 0644)
			fmt.Println("Created .github/workflows/build-and-attest.yml")

			// Generate the slim caller workflow
			callerTmpl := template.Must(template.New("caller").Delims("[[", "]]").Parse(l3CallerTemplate))
			var callerBuf bytes.Buffer
			callerTmpl.Execute(&callerBuf, config)
			os.WriteFile(".github/workflows/secure-release.yml", callerBuf.Bytes(), 0644)
			fmt.Println("Created .github/workflows/secure-release.yml")

			fmt.Println("\nReusable workflow release scaffolding generated.")
			fmt.Println("The caller uses a local same-commit reusable workflow reference.")
			fmt.Println("Run forge harden to pin third-party action refs to SHAs.")
		} else {
			tmpl := template.Must(template.New("workflow").Delims("[[", "]]").Parse(workflowTemplate))
			var buf bytes.Buffer
			tmpl.Execute(&buf, config)
			os.WriteFile(".github/workflows/secure-release.yml", buf.Bytes(), 0644)
			fmt.Println("Created .github/workflows/secure-release.yml")
		}
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVar(&scanGateFlag, "scan-gate", false, "Inject a vulnerability gate step into the workflow")
	initCmd.Flags().BoolVar(&l3Flag, "l3", false, "Generate SLSA Build L3 workflows with reusable workflow isolation")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// initCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
