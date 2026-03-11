/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"encoding/json"
	"os"

	"github.com/spf13/cobra"
)

type Finding struct {
	ID         string
	Module     string
	Version    string
	Severity   string
	Summary    string
	FixedIn    string
	Suppressed bool
}

var severityOrder = map[string]int{
	"critical": 4,
	"high":     3,
	"medium":   2,
	"low":      1,
	"unknown":  0,
}
var osvHTTPClient = &http.Client{Timeout: 5 * time.Second}

func osvURL(id string) string {
	base := strings.TrimRight(os.Getenv("FORGE_OSV_BASE_URL"), "/")
	if base == "" {
		base = "https://api.osv.dev/v1/vulns"
	}
	return base + "/" + id
}

type GovulncheckMessage struct {
	Finding *GovulncheckFinding `json:"finding"`
}

type GovulncheckFinding struct {
	OSV   string `json:"osv"`
	Trace []struct {
		Module  string `json:"module"`
		Version string `json:"version"`
	} `json:"trace"`
}

type OSVResponse struct {
	Summary  string   `json:"summary"`
	Aliases  []string `json:"aliases"`
	Severity []struct {
		Type  string `json:"type"`
		Score string `json:"score"`
	} `json:"severity"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
	Affected []struct {
		Ranges []struct {
			Events []struct {
				Fixed string `json:"fixed"`
			} `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
}

func meetsThreshold(findingSeverity, threshold string) bool {
	return severityOrder[strings.ToLower(findingSeverity)] >= severityOrder[strings.ToLower(threshold)]
}
func runGovulncheck() ([]byte, error) {
	_, checkErr := exec.LookPath("govulncheck")
	if checkErr != nil {
		return nil, fmt.Errorf("\u26A0\uFE0F govulncheck is not downloaded, Install: go install golang.org/x/vuln/cmd/govulncheck@latest")
	}
	out, govErr := exec.Command("govulncheck", "-json", "./...").Output()
	if govErr != nil {
		if len(out) == 0 {
			return nil, fmt.Errorf("govulncheck failed %w", govErr)
		}
	}
	return out, nil

}

func parseGovulncheck(data []byte) []Finding {
	var findings []Finding
	seen := map[string]bool{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var msg GovulncheckMessage
		err := decoder.Decode(&msg)

		if err != nil {
			break
		}

		if msg.Finding == nil || seen[msg.Finding.OSV] {
			continue
		}
		seen[msg.Finding.OSV] = true
		f := msg.Finding

		finding := Finding{
			ID:       f.OSV,
			Severity: "UNKNOWN",
		}

		if len(f.Trace) > 0 {
			finding.Module = f.Trace[0].Module
			finding.Version = f.Trace[0].Version
		}
		enrichFromOSV(&finding)
		findings = append(findings, finding)
	}
	return findings
}

func enrichFromOSV(f *Finding) {
	resp, err := osvHTTPClient.Get(osvURL(f.ID))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var osv OSVResponse
	encoderErr := json.NewDecoder(resp.Body).Decode(&osv)
	if encoderErr != nil {
		return
	}

	f.Summary = osv.Summary
	for _, affected := range osv.Affected {
		for _, r := range affected.Ranges {
			for _, e := range r.Events {
				if e.Fixed != "" {
					f.FixedIn = e.Fixed
				}
			}
		}
	}

	if osv.DatabaseSpecific.Severity != "" {
		f.Severity = osv.DatabaseSpecific.Severity
	}

	for _, alias := range osv.Aliases {
		if strings.HasPrefix(alias, "GHSA-") {
			r2, err := osvHTTPClient.Get(osvURL(alias))
			if err != nil {
				continue
			}
			defer r2.Body.Close()
			var ghsa OSVResponse
			if err := json.NewDecoder(r2.Body).Decode(&ghsa); err != nil {
				continue
			}
			if ghsa.DatabaseSpecific.Severity != "" {
				sev := ghsa.DatabaseSpecific.Severity
				if sev == "MODERATE" {
					sev = "MEDIUM"
				}
				f.Severity = sev
			}
			return
		}
	}

}

func printTextOutput(findings []Finding, threshold string) int {
	blocked := 0
	for _, f := range findings {
		if f.Suppressed {
			continue
		}
		if meetsThreshold(f.Severity, threshold) {
			fmt.Printf("\n  %-10s %s\n", strings.ToUpper(f.Severity), f.ID)
			fmt.Printf("  %-10s %s@%s\n", "Module:", f.Module, f.Version)
			fmt.Printf("  %-10s %s\n", "Summary:", f.Summary)
			fix := "No fix available"
			if f.FixedIn != "" {
				fix = "go get " + f.Module + "@latest"
			}
			fmt.Printf("  %-10s %s\n", "Fix:", fix)
			if meetsThreshold(f.Severity, threshold) {
				blocked++
			}
		}
	}

	fmt.Println()

	if blocked > 0 {
		fmt.Printf("  %d finding(s) at or above threshold (%s). Release blocked.\n", blocked, threshold)
	} else {
		fmt.Println("  No vulnerabilities found above threshold.")
	}
	return blocked
}

func loadSuppressedIDs(path string) map[string]bool {
	IDs := make(map[string]bool)
	file, err := os.ReadFile(path)
	if err != nil {
		return IDs
	}

	data := strings.Split(string(file), "\n")
	for _, l := range data {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		id := strings.TrimSpace(l)
		IDs[id] = true
	}
	return IDs
}

var severityFlag string

// scanCmd represents the scan command
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan dependencies for vulnerabilities and gate releases",
	Long:  `Scan deoendencies`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runGovulncheck()
		if err != nil {
			fmt.Println(err)
			os.Exit(2)
		}
		findings := parseGovulncheck(out)
		suppressed := loadSuppressedIDs(".forgeignore")
		for i := range findings {
			if suppressed[findings[i].ID] {
				findings[i].Suppressed = true
			}
		}
		blocked := printTextOutput(findings, severityFlag)
		if blocked > 0 {
			os.Exit(1)
		}

	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().StringVar(&severityFlag, "severity", "high", "Minimum severity to fail on: critical, high, medium, low")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// scanCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// scanCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
