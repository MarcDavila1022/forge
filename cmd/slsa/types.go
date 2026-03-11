package slsa

const ResultPass = "PASS"
const ResultFail = "FAIL"
const ResultWarn = "WARN"
const slsaSpecVersion = "v1.2"

type BuildTrackCheck struct {
	Name   string `json:"name"`
	Level  string `json:"level"`
	Result string `json:"result"`
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

type HardeningCheck struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Result   string `json:"result"`
	Detail   string `json:"detail,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

type SlsaReport struct {
	WorkflowPath    string
	Mode            string
	BuildLevel      int
	BuildChecks     []BuildTrackCheck
	HardeningChecks []HardeningCheck
	ForgeVersion    string
	ArtifactDigest  string
}

type workflowFile struct {
	Name        string                 `yaml:"name"`
	Permissions map[string]string      `yaml:"permissions"`
	Jobs        map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	RunsOn interface{}    `yaml:"runs-on"`
	Uses   string         `yaml:"uses"`
	Steps  []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
}

type SlsaConfig struct {
	Workflow    string       `yaml:"workflow"`
	TargetLevel int          `yaml:"target_level"`
	Output      string       `yaml:"output"`
	Mode        string       `yaml:"mode"`
	SpecVersion string       `yaml:"spec_version"`
	Verify      VerifyConfig `yaml:"verify"`
}

type VerifyConfig struct {
	DenySelfHostedRunners bool   `yaml:"deny_self_hosted_runners"`
	SignerWorkflow        string `yaml:"signer_workflow"`
	SourceRef             string `yaml:"source_ref"`
}

type ForgeConfigSlsa struct {
	Repo string     `yaml:"repo"`
	Slsa SlsaConfig `yaml:"slsa"`
}

type inTotoStatement struct {
	Type          string      `json:"_type"`
	Subject       []subject   `json:"subject"`
	PredicateType string      `json:"predicateType"`
	Predicate     interface{} `json:"predicate"`
}

type subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

type staticPredicate struct {
	Analyser            analyserInfo      `json:"analyser"`
	TimeAnalysed        string            `json:"timeAnalysed"`
	SpecVersion         string            `json:"specVersion"`
	EstimatedBuildLevel string            `json:"estimatedBuildLevel"`
	BuildTrackChecks    []BuildTrackCheck `json:"buildTrackChecks"`
	HardeningChecks     []HardeningCheck  `json:"hardeningChecks"`
}

type analyserInfo struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type vsaPredicate struct {
	Verifier           vsaVerifier     `json:"verifier"`
	TimeVerified       string          `json:"timeVerified"`
	ResourceURI        string          `json:"resourceUri"`
	Policy             vsaPolicy       `json:"policy"`
	InputAttestations  []interface{}   `json:"inputAttestations"`
	VerificationResult string          `json:"verificationResult"`
	VerifiedLevels     []string        `json:"verifiedLevels"`
	SlsaVersion        string          `json:"slsaVersion"`
	ForgeExtensions    forgeExtensions `json:"https://forge.dev/extensions/v1"`
}

type vsaVerifier struct {
	ID      string            `json:"id"`
	Version map[string]string `json:"version"`
}

type vsaPolicy struct {
	URI    string            `json:"uri"`
	Digest map[string]string `json:"digest"`
}

type forgeExtensions struct {
	AnalysisMode           string            `json:"analysisMode"`
	SelfHostedRunnerDenied bool              `json:"selfHostedRunnerDenied"`
	BuildTrackChecks       []BuildTrackCheck `json:"buildTrackChecks"`
}
