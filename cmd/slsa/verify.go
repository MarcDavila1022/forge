package slsa

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func buildVerifyArgs(artifact string, repo string, signerWorkflow string, sourceRef string, denySelfHosted bool) []string {
	args := []string{"attestation", "verify", artifact}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	if denySelfHosted {
		args = append(args, "--deny-self-hosted-runners")
	}
	if signerWorkflow != "" {
		args = append(args, "--signer-workflow", signerWorkflow)
	}
	if sourceRef != "" {
		args = append(args, "--source-ref", sourceRef)
	}
	return append(args, "--format", "json")
}

func runVerifyMode(path string, artifact string, repo string) (*SlsaReport, error) {
	if artifact == "" {
		return nil, fmt.Errorf("verify mode requires --artifact <path>. Use --mode static for config-only analysis")
	}
	artifactHash, hashErr := sha256File(artifact)
	if hashErr != nil {
		return nil, fmt.Errorf("could not hash artifact: %w", hashErr)
	}

	report, err := runStaticAnalysis(path, repo)
	if err != nil {
		return nil, err
	}
	report.Mode = "verify"
	report.ArtifactDigest = artifactHash

	verifyArgs := buildVerifyArgs(artifact, repo, slsaSignerWorkflow, slsaSourceRef, slsaDenySelfHosted)

	out, verifyErr := exec.Command("gh", verifyArgs...).CombinedOutput()

	verifyChecks := append([]BuildTrackCheck{}, report.BuildChecks...)

	attestResult := ResultFail
	attestDetail := "Attestation verification failed"
	if verifyErr == nil {
		attestResult = ResultPass
		attestDetail = "Attestation verified successfully"
	} else {
		attestDetail = fmt.Sprintf("Attestation verification failed: %s", strings.TrimSpace(string(out)))
	}
	verifyChecks = append(verifyChecks, BuildTrackCheck{
		Name: "attestation_valid", Level: "L2", Result: attestResult, Detail: attestDetail,
	})

	predicateResult := ResultFail
	predicateDetail := "Could not verify predicate type"
	if verifyErr == nil && len(out) > 0 {
		if strings.Contains(string(out), "https://slsa.dev/provenance/v1") ||
			strings.Contains(string(out), "https://slsa.dev/provenance/v0") {
			predicateResult = ResultPass
			predicateDetail = "Provenance predicate type correct"
		} else {
			predicateDetail = "Attestation does not use SLSA provenance predicate type"
		}
	}
	verifyChecks = append(verifyChecks, BuildTrackCheck{
		Name: "provenance_predicate_correct", Level: "L2", Result: predicateResult, Detail: predicateDetail,
	})

	selfHostedResult := ResultFail
	selfHostedDetail := "Verification did not enforce hosted runner policy; rerun with --deny-self-hosted-runners"
	if slsaDenySelfHosted {
		selfHostedResult = ResultPass
		selfHostedDetail = "Attestation verified with self-hosted runners denied"
		if verifyErr != nil && strings.Contains(string(out), "self-hosted") {
			selfHostedResult = ResultFail
			selfHostedDetail = "Attestation generated on self-hosted runner"
		}
	}
	verifyChecks = append(verifyChecks, BuildTrackCheck{
		Name: "runner_policy_enforced", Level: "L2", Result: selfHostedResult, Detail: selfHostedDetail,
	})

	signerResult := ResultFail
	signerDetail := "Use --signer-workflow to verify the exact build workflow"
	if slsaSignerWorkflow != "" {
		if verifyErr == nil {
			signerResult = ResultPass
			signerDetail = "Signer workflow matches: " + slsaSignerWorkflow
		} else {
			signerResult = ResultFail
			signerDetail = "Signer workflow verification failed"
		}
	}
	verifyChecks = append(verifyChecks, BuildTrackCheck{
		Name: "signer_workflow_matches", Level: "L3", Result: signerResult, Detail: signerDetail,
	})

	sourceRefResult := ResultFail
	sourceRefDetail := "Use --source-ref to verify the exact source ref used for the build"
	if slsaSourceRef != "" {
		if verifyErr == nil {
			sourceRefResult = ResultPass
			sourceRefDetail = "Source ref matches: " + slsaSourceRef
		} else {
			sourceRefResult = ResultFail
			sourceRefDetail = "Source ref verification failed"
		}
	}
	verifyChecks = append(verifyChecks, BuildTrackCheck{
		Name: "source_ref_matches", Level: "L3", Result: sourceRefResult, Detail: sourceRefDetail,
	})

	report.BuildChecks = verifyChecks
	report.BuildLevel = computeBuildLevel(verifyChecks)

	for i := range report.HardeningChecks {
		report.HardeningChecks[i].Detail += " (from workflow: " + path + ")"
	}

	return report, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
