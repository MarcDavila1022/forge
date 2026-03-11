package slsa

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

func parseWorkflowFile(path string) (*workflowFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wf workflowFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}
	return &wf, nil
}

func hasAttestAction(steps []workflowStep) bool {
	for _, s := range steps {
		if matchesAttestAction(s.Uses) {
			return true
		}
	}
	return false
}

func matchesAttestAction(uses string) bool {
	if uses == "" {
		return false
	}
	name := strings.Split(uses, "@")[0]
	return name == "actions/attest-build-provenance" ||
		name == "actions/attest" ||
		strings.HasPrefix(name, "slsa-framework/slsa-github-generator")
}

func hasBuildSteps(steps []workflowStep) bool {
	for _, s := range steps {
		if s.Run != "" {
			return true
		}
	}
	return false
}

var ghHostedPatterns = []string{
	"ubuntu-", "macos-", "windows-",
}
var shaRefRegex = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func isGitHubHostedRunner(runsOn interface{}) (hosted bool, selfHosted bool) {
	var runner string
	switch v := runsOn.(type) {
	case string:
		runner = v
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				if s == "self-hosted" {
					return false, true
				}
			}
		}
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				runner = s
			}
		}
	default:
		return false, false
	}

	if runner == "self-hosted" {
		return false, true
	}
	for _, prefix := range ghHostedPatterns {
		if strings.HasPrefix(runner, prefix) {
			return true, false
		}
	}
	return false, false
}

func hasRequiredPermissions(perms map[string]string) bool {
	idToken, ok1 := perms["id-token"]
	attestations, ok2 := perms["attestations"]
	return ok1 && ok2 && idToken == "write" && attestations == "write"
}

func isReusableWorkflowRef(uses string) bool {
	return uses != "" && (strings.Contains(uses, ".yml") || strings.Contains(uses, ".yaml"))
}

func isImmutableExternalRef(uses string) bool {
	if uses == "" {
		return true
	}
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "docker://") {
		return true
	}
	parts := strings.SplitN(uses, "@", 2)
	return len(parts) == 2 && shaRefRegex.MatchString(parts[1])
}

func immutableReusableWorkflowRef(uses string) (bool, string) {
	if strings.HasPrefix(uses, "./.github/workflows/") {
		return true, "Reusable workflow uses a local same-commit reference"
	}
	parts := strings.SplitN(uses, "@", 2)
	if len(parts) == 2 && shaRefRegex.MatchString(parts[1]) {
		return true, "Reusable workflow ref pinned to SHA"
	}
	if len(parts) == 2 {
		return false, fmt.Sprintf("Reusable workflow ref uses mutable ref @%s", parts[1])
	}
	return false, "Reusable workflow ref is missing an immutable reference"
}

func checkReusableWorkflowIsolation(wf *workflowFile) (reusable bool, attestInReusable bool) {
	for _, job := range wf.Jobs {
		if isReusableWorkflowRef(job.Uses) {
			reusable = true
			if !hasAttestAction(job.Steps) {
				attestInReusable = true
			}
		}
	}
	for _, job := range wf.Jobs {
		if hasAttestAction(job.Steps) && !isReusableWorkflowRef(job.Uses) {
			attestInReusable = false
		}
	}
	return
}

func countMutableExternalRefs(wf *workflowFile) int {
	mutable := 0
	for _, job := range wf.Jobs {
		if job.Uses != "" && !isImmutableExternalRef(job.Uses) {
			mutable++
		}
		for _, step := range job.Steps {
			if step.Uses == "" {
				continue
			}
			if !isImmutableExternalRef(step.Uses) {
				mutable++
			}
		}
	}
	return mutable
}

var hermeticDangerPatterns = []string{"curl ", "curl\t", "wget ", "wget\t", "http://", "https://"}
var hermeticAllowedPrefixes = []string{"go install", "npm install", "pip install", "apt-get", "brew install", "apk add"}

func checkHermeticBuild(wf *workflowFile) string {
	for _, job := range wf.Jobs {
		for _, step := range job.Steps {
			if step.Run == "" {
				continue
			}
			lower := strings.ToLower(step.Run)
			for _, pattern := range hermeticDangerPatterns {
				if strings.Contains(lower, pattern) {

					allowed := false
					for _, prefix := range hermeticAllowedPrefixes {
						if strings.Contains(lower, prefix) {
							allowed = true
							break
						}
					}
					if !allowed {
						return ResultWarn
					}
				}
			}
		}
	}
	return ResultPass
}

func getDefaultBranch(repo string) string {
	out, err := exec.Command("gh", "api",
		fmt.Sprintf("/repos/%s", repo),
		"--jq", ".default_branch").CombinedOutput()
	if err != nil {
		return "main"
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "main"
	}
	return branch
}

func checkBranchProtection(repo string) (protected bool, branch string, err error) {
	if repo == "" {
		return false, "", fmt.Errorf("no repo configured")
	}
	branch = getDefaultBranch(repo)
	out, apiErr := exec.Command("gh", "api",
		fmt.Sprintf("/repos/%s/branches/%s/protection", repo, branch),
		"--jq", ".required_pull_request_reviews.required_approving_review_count").CombinedOutput()
	if apiErr != nil {
		outStr := strings.TrimSpace(string(out))
		// Degrade gracefully: 404 means not protected or insufficient permissions
		if strings.Contains(outStr, "Not Found") || strings.Contains(outStr, "Branch not protected") {
			return false, branch, nil
		}
		if strings.Contains(outStr, "Must have admin access") || strings.Contains(outStr, "403") {
			return false, branch, fmt.Errorf("insufficient permissions to check branch protection (requires admin scope)")
		}
		return false, branch, fmt.Errorf("could not check branch protection: %s", outStr)
	}
	count := strings.TrimSpace(string(out))
	return count != "" && count != "0" && count != "null", branch, nil
}

func resolveReusableWorkflowPath(uses string) string {
	if strings.HasPrefix(uses, "./") {
		return strings.TrimPrefix(uses, "./")
	}
	atIdx := strings.Index(uses, "@")
	if atIdx == -1 {
		return ""
	}
	full := uses[:atIdx]
	ghIdx := strings.Index(full, ".github/workflows/")
	if ghIdx == -1 {
		return ""
	}
	return full[ghIdx:]
}

func collectWorkflowChecks(wf *workflowFile) (scripted, provenance, hosted, selfHosted, verifiable bool, runnerDetail string, provDetail string) {
	for _, job := range wf.Jobs {
		if hasBuildSteps(job.Steps) {
			scripted = true
		}
		if hasAttestAction(job.Steps) {
			provenance = true
			for _, step := range job.Steps {
				if matchesAttestAction(step.Uses) {
					name := strings.Split(step.Uses, "@")[0]
					provDetail = name + " detected"
				}
			}
		}
		h, sh := isGitHubHostedRunner(job.RunsOn)
		if h {
			hosted = true
			if runner, ok := job.RunsOn.(string); ok {
				runnerDetail = runner + " runner detected"
			}
		}
		if sh {
			selfHosted = true
		}
	}
	if provenance && hasRequiredPermissions(wf.Permissions) {
		verifiable = true
	}
	return
}

func runStaticAnalysis(path string, repo string) (*SlsaReport, error) {
	wf, err := parseWorkflowFile(path)
	if err != nil {
		return nil, err
	}

	report := &SlsaReport{
		WorkflowPath: path,
		Mode:         "static",
	}

	hasScriptedBuild, hasProvenance, hostedBuild, selfHostedDetected, provenanceVerifiable, runnerDetail, provDetail := collectWorkflowChecks(wf)

	reusable, attestInReusable := checkReusableWorkflowIsolation(wf)

	for _, job := range wf.Jobs {
		if !isReusableWorkflowRef(job.Uses) {
			continue
		}
		localPath := resolveReusableWorkflowPath(job.Uses)
		if localPath == "" {
			continue
		}
		if _, statErr := os.Stat(localPath); statErr != nil {
			continue
		}
		reusableWf, parseErr := parseWorkflowFile(localPath)
		if parseErr != nil {
			continue
		}
		rScripted, rProv, rHosted, rSelfHosted, rVerifiable, rRunnerDetail, rProvDetail := collectWorkflowChecks(reusableWf)
		if rScripted {
			hasScriptedBuild = true
		}
		if rProv {
			hasProvenance = true
			if rProvDetail != "" {
				provDetail = rProvDetail
			}
		}
		if rHosted {
			hostedBuild = true
			if rRunnerDetail != "" {
				runnerDetail = rRunnerDetail
			}
		}
		if rSelfHosted {
			selfHostedDetected = true
		}
		if rVerifiable {
			provenanceVerifiable = true
		}
	}

	scriptedResult := ResultFail
	scriptedDetail := "No workflow file or build steps found"
	if hasScriptedBuild {
		scriptedResult = ResultPass
		scriptedDetail = "Workflow file present with build steps"
	}
	report.BuildChecks = append(report.BuildChecks, BuildTrackCheck{
		Name: "scripted_build", Level: "L1", Result: scriptedResult, Detail: scriptedDetail,
	})

	provResult := ResultFail
	if provDetail == "" {
		provDetail = "No attestation action detected"
	}
	if hasProvenance {
		provResult = ResultPass
		if provDetail == "" {
			provDetail = "Attestation action detected"
		}
	}
	report.BuildChecks = append(report.BuildChecks, BuildTrackCheck{
		Name: "provenance_generated", Level: "L1", Result: provResult, Detail: provDetail,
	})

	hostedResult := ResultFail
	hostedDetail := "No GitHub-hosted runner detected"
	if hostedBuild {
		hostedResult = ResultPass
		if runnerDetail != "" {
			hostedDetail = runnerDetail
		} else {
			hostedDetail = "GitHub-hosted runner detected"
		}
	} else if selfHostedDetected {
		hostedResult = ResultWarn
		hostedDetail = "Self-hosted runner detected; Build L2 depends on provenance generation boundary"
	}
	report.BuildChecks = append(report.BuildChecks, BuildTrackCheck{
		Name: "hosted_build", Level: "L2", Result: hostedResult, Detail: hostedDetail,
	})

	verifiableResult := ResultFail
	verifiableDetail := "Attest action or required permissions (id-token: write, attestations: write) missing"
	if provenanceVerifiable {
		verifiableResult = ResultPass
		verifiableDetail = "Attest action + id-token: write + attestations: write"
	}
	report.BuildChecks = append(report.BuildChecks, BuildTrackCheck{
		Name: "provenance_verifiable", Level: "L2", Result: verifiableResult, Detail: verifiableDetail,
	})

	reusableResult := ResultFail
	reusableDetail := "Build + attest steps are in the caller workflow"
	reusableFix := "Run: forge init --l3 to generate L3 workflows with reusable workflow isolation"
	if reusable {
		reusableResult = ResultPass
		reusableDetail = "Build delegated to reusable workflow"
		reusableFix = ""
	}
	report.BuildChecks = append(report.BuildChecks, BuildTrackCheck{
		Name: "reusable_workflow_isolation", Level: "L3", Result: reusableResult, Detail: reusableDetail, Fix: reusableFix,
	})

	attestReusableResult := ResultFail
	attestReusableDetail := "Attest action must run inside the reusable workflow"
	attestReusableFix := "Run: forge init --l3 to generate L3 workflows with reusable workflow isolation"
	if attestInReusable {
		attestReusableResult = ResultPass
		attestReusableDetail = "Attest action runs inside reusable workflow"
		attestReusableFix = ""
	}
	report.BuildChecks = append(report.BuildChecks, BuildTrackCheck{
		Name: "attestation_in_reusable_workflow", Level: "L3", Result: attestReusableResult, Detail: attestReusableDetail, Fix: attestReusableFix,
	})

	immutableReusableResult := ResultFail
	immutableReusableDetail := "Reusable workflow reference must use a local path or a commit SHA"
	immutableReusableFix := "Use ./.github/workflows/... for same-repo workflows or pin remote reusable workflow refs to a commit SHA"
	seenReusable := false
	allReusableImmutable := true
	for _, job := range wf.Jobs {
		if !isReusableWorkflowRef(job.Uses) {
			continue
		}
		seenReusable = true
		immutable, detail := immutableReusableWorkflowRef(job.Uses)
		if !immutable {
			allReusableImmutable = false
			immutableReusableDetail = detail
			break
		}
		immutableReusableDetail = detail
	}
	if seenReusable && allReusableImmutable {
		immutableReusableResult = ResultPass
		immutableReusableFix = ""
	} else if !seenReusable {
		immutableReusableDetail = "No reusable workflow detected — Build L3 requires reusable workflow isolation"
		immutableReusableFix = "Run: forge init --l3 to generate reusable workflow isolation"
	}
	report.BuildChecks = append(report.BuildChecks, BuildTrackCheck{
		Name: "reusable_workflow_reference_immutable", Level: "L3", Result: immutableReusableResult, Detail: immutableReusableDetail, Fix: immutableReusableFix,
	})

	allWorkflows := []*workflowFile{wf}
	for _, job := range wf.Jobs {
		if !isReusableWorkflowRef(job.Uses) {
			continue
		}
		localPath := resolveReusableWorkflowPath(job.Uses)
		if localPath == "" {
			continue
		}
		if rwf, err := parseWorkflowFile(localPath); err == nil {
			allWorkflows = append(allWorkflows, rwf)
		}
	}

	totalMutableRefs := 0
	for _, w := range allWorkflows {
		totalMutableRefs += countMutableExternalRefs(w)
	}
	pinResult := ResultPass
	pinDetail := "All external action and workflow refs are immutable"
	pinFix := ""
	if totalMutableRefs > 0 {
		pinResult = ResultFail
		pinDetail = fmt.Sprintf("%d mutable external refs detected", totalMutableRefs)
		pinFix = "Run: forge harden to fix"
	}
	report.HardeningChecks = append(report.HardeningChecks, HardeningCheck{
		Name: "external_refs_immutable", Category: "action_integrity", Result: pinResult, Detail: pinDetail, Fix: pinFix,
	})

	bpResult := ResultWarn
	bpDetail := "Could not check branch protection"
	bpFix := ""
	if repo != "" {
		protected, branch, bpErr := checkBranchProtection(repo)
		if bpErr != nil {
			bpDetail = "Could not check: " + bpErr.Error()
		} else if protected {
			bpResult = ResultPass
			bpDetail = fmt.Sprintf("Required PR reviews enforced on %s", branch)
		} else {
			bpResult = ResultFail
			bpDetail = fmt.Sprintf("Required PR reviews not enforced on %s", branch)
			bpFix = "Fix: Settings → Branches → Require pull request reviews"
		}
	}
	report.HardeningChecks = append(report.HardeningChecks, HardeningCheck{
		Name: "branch_protection", Category: "source_integrity", Result: bpResult, Detail: bpDetail, Fix: bpFix,
	})

	hermeticResult := ResultPass
	for _, w := range allWorkflows {
		if r := checkHermeticBuild(w); r == ResultWarn {
			hermeticResult = ResultWarn
		}
	}
	hermeticDetail := "No external downloads detected in build steps"
	if hermeticResult == ResultWarn {
		hermeticDetail = "Cannot verify statically — review workflow for external curl/wget calls outside of build steps"
	}
	report.HardeningChecks = append(report.HardeningChecks, HardeningCheck{
		Name: "hermetic_build", Category: "build_hygiene", Result: hermeticResult, Detail: hermeticDetail,
	})

	report.BuildLevel = computeBuildLevel(report.BuildChecks)

	return report, nil
}

func computeBuildLevel(checks []BuildTrackCheck) int {
	level := 0
	l1Pass := true
	l2Pass := true
	l3Pass := true
	for _, c := range checks {
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
	if l1Pass {
		level = 1
	}
	if l1Pass && l2Pass {
		level = 2
	}
	if l1Pass && l2Pass && l3Pass {
		level = 3
	}
	return level
}
