package slsa

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func checkSymbol(result string) string {
	switch result {
	case ResultPass:
		return "✓"
	case ResultFail:
		return "✗"
	case ResultWarn:
		return "~"
	}
	return "?"
}

func targetLevel(report *SlsaReport, target int) int {
	nextLevel := report.BuildLevel + 1
	if nextLevel > 3 {
		nextLevel = 3
	}
	if target > 0 {
		nextLevel = target
	}
	if nextLevel < 1 {
		nextLevel = 1
	}
	if nextLevel > 3 {
		nextLevel = 3
	}
	return nextLevel
}

func modeSummary(mode string) string {
	switch mode {
	case "static":
		return "static (workflow estimate)"
	case "verify":
		return "verify (artifact-backed)"
	default:
		return mode
	}
}

func levelLabel(mode string) string {
	if mode == "verify" {
		return "Verified SLSA Build Level"
	}
	return "Estimated SLSA Build Level"
}

func printSummarySection(report *SlsaReport, target int) int {
	nextLevel := targetLevel(report, target)

	fmt.Println("Summary")
	fmt.Printf("  Workflow: %s\n", report.WorkflowPath)
	fmt.Printf("  Mode: %s\n", modeSummary(report.Mode))
	fmt.Printf("  %s: L%d\n", levelLabel(report.Mode), report.BuildLevel)
	fmt.Printf("  Evaluating toward: Build L%d\n", nextLevel)
	if report.ArtifactDigest != "" {
		fmt.Printf("  Artifact SHA256: %s\n", report.ArtifactDigest)
	}
	fmt.Println()

	return nextLevel
}

func printBuildLevelSection(report *SlsaReport, label string, prefix string) {
	fmt.Println(label)
	printed := false
	for _, c := range report.BuildChecks {
		if c.Level != prefix {
			continue
		}
		printed = true
		fmt.Printf("  %s %-35s %s\n", checkSymbol(c.Result), c.Name, c.Detail)
		if c.Fix != "" && c.Result == ResultFail {
			fmt.Printf("    Fix: %s\n", c.Fix)
		}
	}
	if !printed {
		fmt.Println("  No checks recorded.")
	}
	fmt.Println()
}

func printHardeningSection(report *SlsaReport) {
	fmt.Println("Recommended Hardening")
	for _, c := range report.HardeningChecks {
		fmt.Printf("  %s %-35s %s\n", checkSymbol(c.Result), c.Name, c.Detail)
		if c.Fix != "" && c.Result == ResultFail {
			fmt.Printf("    %s\n", c.Fix)
		}
	}
	fmt.Println()
}

func printTextReport(report *SlsaReport, target int) {
	fmt.Printf("\nforge slsa ▸ Analysing %s (%s mode)\n\n", report.WorkflowPath, report.Mode)
	nextLevel := printSummarySection(report, target)

	fmt.Println("Build Track")
	fmt.Println()
	printBuildLevelSection(report, "Build L1", "L1")
	printBuildLevelSection(report, "Build L2", "L2")
	printBuildLevelSection(report, "Build L3", "L3")

	printHardeningSection(report)

	if report.Mode == "static" {
		fmt.Println("⚠ Static analysis — estimated capability from workflow config.")
		fmt.Println("  Use --mode verify --artifact <path> to verify a real artifact.")
		fmt.Println()
	}

	blocking := 0
	for _, c := range report.BuildChecks {
		if c.Result == ResultFail && c.Level == fmt.Sprintf("L%d", nextLevel) {
			blocking++
		}
	}
	if blocking > 0 {
		fmt.Printf("%d Build track requirement(s) blocking L%d.\n", blocking, nextLevel)
		fmt.Printf("Run forge slsa --target %d for a remediation plan.\n", nextLevel)
	} else if report.BuildLevel >= nextLevel {
		fmt.Printf("%s L%d achieved.\n", strings.TrimSuffix(levelLabel(report.Mode), " Level"), report.BuildLevel)
	}
}

func printBadge(level int) {
	fmt.Printf("![SLSA Build L%d](https://img.shields.io/badge/SLSA_Build-L%d-4FC3F7?style=flat-square&logo=slsa)\n", level, level)
}

func printMarkdownReport(report *SlsaReport) {
	fmt.Println("# SLSA Build Track Compliance Report")
	fmt.Println()
	fmt.Printf("**%s:** L%d\n", levelLabel(report.Mode), report.BuildLevel)
	modeTitle := strings.ToUpper(report.Mode[:1]) + report.Mode[1:]
	fmt.Printf("**Analysis Mode:** %s\n", modeTitle)
	fmt.Printf("**Spec Version:** SLSA %s\n", slsaSpecVersion)
	fmt.Printf("**Generated:** %s by forge slsa %s\n", time.Now().Format("2006-01-02"), report.ForgeVersion)
	fmt.Println()

	fmt.Println("## Build Track Requirements")

	l1Pass := true
	l2Pass := true
	l3Pass := true
	for _, c := range report.BuildChecks {
		if c.Result == ResultFail {
			switch c.Level {
			case "L1":
				l1Pass = false
			case "L2":
				l2Pass = false
			case "L3":
				l3Pass = false
			}
		}
	}

	l1Icon := "❌"
	if l1Pass {
		l1Icon = "✅"
	}
	l2Icon := "❌"
	if l2Pass {
		l2Icon = "✅"
	}

	l3Icon := "❌"
	if l3Pass {
		l3Icon = "✅"
	}

	fmt.Printf("- %s Build L1: Scripted build + provenance generated\n", l1Icon)
	fmt.Printf("- %s Build L2: Hosted platform + verifiable signed provenance\n", l2Icon)
	fmt.Printf("- %s Build L3: Reusable workflow isolation\n", l3Icon)
	fmt.Println()

	fmt.Println("## Recommended Hardening (non-SLSA)")
	for _, c := range report.HardeningChecks {
		icon := "✅"
		if c.Result == ResultFail {
			icon = "❌"
		} else if c.Result == ResultWarn {
			icon = "⚠️"
		}
		fmt.Printf("- %s %s: %s\n", icon, c.Name, c.Detail)
	}
}

func printJSONStatic(report *SlsaReport, repo string) {
	stmt := inTotoStatement{
		Type: "https://in-toto.io/Statement/v1",
		Subject: []subject{
			{Name: report.WorkflowPath, Digest: map[string]string{}},
		},
		PredicateType: "https://forge.dev/slsa-static-report/v1",
		Predicate: staticPredicate{
			Analyser: analyserInfo{
				ID:      "https://github.com/" + repo,
				Version: report.ForgeVersion,
			},
			TimeAnalysed:        time.Now().UTC().Format(time.RFC3339),
			SpecVersion:         "https://slsa.dev/spec/" + slsaSpecVersion,
			EstimatedBuildLevel: fmt.Sprintf("SLSA_BUILD_LEVEL_%d", report.BuildLevel),
			BuildTrackChecks:    report.BuildChecks,
			HardeningChecks:     report.HardeningChecks,
		},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(stmt)
}

func printJSONVerify(report *SlsaReport, repo string, artifact string) {
	digest := map[string]string{}
	if report.ArtifactDigest != "" {
		digest["sha256"] = report.ArtifactDigest
	}

	verResult := "PASSED"
	for _, c := range report.BuildChecks {
		if c.Result == ResultFail {
			verResult = "FAILED"
			break
		}
	}

	verifiedLevels := []string{fmt.Sprintf("SLSA_BUILD_LEVEL_%d", report.BuildLevel)}

	var extChecks []BuildTrackCheck
	for _, c := range report.BuildChecks {
		if c.Level != "L1" {
			extChecks = append(extChecks, c)
		}
	}

	ghVersion := ""
	if out, err := exec.Command("gh", "--version").Output(); err == nil {
		parts := strings.Fields(string(out))
		for i, p := range parts {
			if p == "version" && i+1 < len(parts) {
				ghVersion = parts[i+1]
				break
			}
		}
	}

	stmt := inTotoStatement{
		Type: "https://in-toto.io/Statement/v1",
		Subject: []subject{
			{Name: artifact, Digest: digest},
		},
		PredicateType: "https://slsa.dev/verification_summary/v1",
		Predicate: vsaPredicate{
			Verifier: vsaVerifier{
				ID: "https://github.com/" + repo,
				Version: map[string]string{
					"forge": report.ForgeVersion,
					"gh":    ghVersion,
				},
			},
			TimeVerified:       time.Now().UTC().Format(time.RFC3339),
			ResourceURI:        "git+https://github.com/" + repo,
			Policy:             vsaPolicy{URI: "https://slsa.dev/spec/" + slsaSpecVersion, Digest: map[string]string{}},
			InputAttestations:  []interface{}{},
			VerificationResult: verResult,
			VerifiedLevels:     verifiedLevels,
			SlsaVersion:        "1.2",
			ForgeExtensions: forgeExtensions{
				AnalysisMode:           "verify",
				SelfHostedRunnerDenied: slsaDenySelfHosted,
				BuildTrackChecks:       extChecks,
			},
		},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(stmt)
}
