package slsa

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	slsaWorkflow       string
	slsaOutput         string
	slsaTarget         int
	slsaBadge          bool
	slsaMode           string
	slsaArtifact       string
	slsaDenySelfHosted bool
	slsaSignerWorkflow string
	slsaSourceRef      string
	forgeVersion       string
)

// slsaCmd represents the slsa command
var slsaCmd = &cobra.Command{
	Use:   "slsa",
	Short: "Analyse SLSA Build level compliance for your release workflow",
	Long: `Analyse the repository's release workflow and report what SLSA Build level
it currently achieves, what requirements are met or unmet, and what steps
would advance it to the next level.

Supports two modes:
  static  — inspect workflow YAML and repo configuration (default)
  verify  — verify a real artifact's attestation via gh CLI`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load forge.yml config for defaults
		var config ForgeConfigSlsa
		if data, err := os.ReadFile("forge.yml"); err == nil {
			yaml.Unmarshal(data, &config)
		}

		repo := config.Repo
		if !cmd.Flags().Changed("workflow") && config.Slsa.Workflow != "" {
			slsaWorkflow = config.Slsa.Workflow
		}
		if !cmd.Flags().Changed("output") && config.Slsa.Output != "" {
			slsaOutput = config.Slsa.Output
		}
		if !cmd.Flags().Changed("mode") && config.Slsa.Mode != "" {
			slsaMode = config.Slsa.Mode
		}
		if !cmd.Flags().Changed("target") && config.Slsa.TargetLevel > 0 {
			slsaTarget = config.Slsa.TargetLevel
		}
		if !cmd.Flags().Changed("deny-self-hosted-runners") && config.Slsa.Verify.DenySelfHostedRunners {
			slsaDenySelfHosted = true
		}
		if !cmd.Flags().Changed("signer-workflow") && config.Slsa.Verify.SignerWorkflow != "" {
			slsaSignerWorkflow = config.Slsa.Verify.SignerWorkflow
		}
		if !cmd.Flags().Changed("source-ref") && config.Slsa.Verify.SourceRef != "" {
			slsaSourceRef = config.Slsa.Verify.SourceRef
		}
		if _, err := os.Stat(slsaWorkflow); os.IsNotExist(err) {
			fmt.Printf("Workflow not found at %s. Use --workflow to specify.\n", slsaWorkflow)
			os.Exit(2)
		}
		if slsaMode != "static" && slsaMode != "verify" {
			fmt.Printf("Invalid mode: %s. Use 'static' or 'verify'.\n", slsaMode)
			os.Exit(2)
		}
		if slsaMode == "verify" && slsaArtifact == "" {
			fmt.Println("Verify mode requires --artifact <path>. Use --mode static for config-only analysis.")
			os.Exit(2)
		}

		var report *SlsaReport
		var err error

		if slsaMode == "verify" {
			report, err = runVerifyMode(slsaWorkflow, slsaArtifact, repo)
		} else {
			report, err = runStaticAnalysis(slsaWorkflow, repo)
		}
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(2)
		}
		report.ForgeVersion = forgeVersion
		if slsaBadge {
			printBadge(report.BuildLevel)
			return
		}
		switch slsaOutput {
		case "json":
			if report.Mode == "verify" {
				printJSONVerify(report, repo, slsaArtifact)
			} else {
				printJSONStatic(report, repo)
			}
		case "markdown":
			printMarkdownReport(report)
		default:
			printTextReport(report, slsaTarget)
		}
	},
}

func NewCommand(version string) *cobra.Command {
	forgeVersion = version
	slsaCmd.Flags().StringVar(&slsaWorkflow, "workflow", ".github/workflows/secure-release.yml", "Path to workflow file to analyse")
	slsaCmd.Flags().StringVar(&slsaOutput, "output", "text", "Output format: text, json, markdown")
	slsaCmd.Flags().IntVar(&slsaTarget, "target", 0, "Target Build level to evaluate gaps against (1, 2, 3)")
	slsaCmd.Flags().BoolVar(&slsaBadge, "badge", false, "Print a Shields.io badge markdown string")
	slsaCmd.Flags().StringVar(&slsaMode, "mode", "static", "Analysis mode: static or verify")
	slsaCmd.Flags().StringVar(&slsaArtifact, "artifact", "", "Artifact path for verify mode")
	slsaCmd.Flags().BoolVar(&slsaDenySelfHosted, "deny-self-hosted-runners", false, "Fail if attestation was generated on a self-hosted runner (verify mode)")
	slsaCmd.Flags().StringVar(&slsaSignerWorkflow, "signer-workflow", "", "Expected signer workflow for L3 verification (verify mode)")
	slsaCmd.Flags().StringVar(&slsaSourceRef, "source-ref", "", "Expected source ref for extra strictness (verify mode)")

	return slsaCmd
}
