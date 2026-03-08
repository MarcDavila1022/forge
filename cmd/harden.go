/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"path"

	"os"

	"strings"

	"regexp"

	"net/http"

	"encoding/json"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type ActionRef struct {
	Value string
	Line  int
}

type RefKind int

const (
	KindPin RefKind = iota
	KindLocal
	KindDocker
	KindReusable
	KindAction
)

type GitRefResponse struct {
	Object struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	} `json:"object"`
}

var shaCache = map[string]string{}

func directoryFind(dir string) ([]string, error) {
	var lis []string
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("No workflow files found in %s", dir)
	}

	for _, file := range files {
		yml := strings.HasSuffix(file.Name(), ".yml")
		yaml := strings.HasSuffix(file.Name(), ".yaml")

		if yml || yaml {
			lis = append(lis, dir+"/"+file.Name())
		}
	}

	if len(lis) == 0 {
		return nil, fmt.Errorf("no workflow files found in %s", dir)
	}

	return lis, nil

}

func parseWorkflowHelper(node *yaml.Node, refs *[]ActionRef) {
	if node.Kind == yaml.MappingNode {

		for i := 0; i < len(node.Content)-1; i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Value == "uses" {
				ref := ActionRef{
					Value: value.Value,
					Line:  value.Line,
				}
				*refs = append(*refs, ref)
			}
		}
	}
	for _, child := range node.Content {
		parseWorkflowHelper(child, refs)
	}

}

func parseWorkflow(path string) ([]ActionRef, error) {
	var usesFinding []ActionRef
	file, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, readErr
	}
	var node yaml.Node
	treeErr := yaml.Unmarshal(file, &node)

	if treeErr != nil {
		return nil, treeErr
	}

	parseWorkflowHelper(&node, &usesFinding)
	return usesFinding, nil

}

func category(ref ActionRef) RefKind {
	if strings.HasPrefix(ref.Value, "./") {
		return KindLocal
	} else if strings.HasPrefix(ref.Value, "docker://") {
		return KindDocker
	} else if strings.Contains(ref.Value, ".github/workflows/") {
		return KindReusable
	}
	parts := strings.Split(ref.Value, "@")
	if len(parts) >= 2 {
		matched, _ := regexp.MatchString(`^[0-9a-fA-F]{40}$`, parts[1])
		if matched {
			return KindPin
		}
	}
	return KindAction
}

func shouldSkip(value string, skipList string) bool {
	name := strings.Split(value, "@")[0]
	patterns := strings.Split(skipList, ",")
	for _, pattern := range patterns {
		matched, _ := path.Match(pattern, name)
		if matched {
			return true
		}
	}
	return false
}

func githubGet(url string) (*http.Response, error) {
	token := os.Getenv("GH_TOKEN")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{}
	return client.Do(req)

}

func resolveRefToSHA(value string) (string, error) {
	v, ok := shaCache[value]
	if ok {
		return v, nil
	}
	firstSplit := strings.Split(value, "@")
	reference := firstSplit[1]
	secondSplit := strings.Split(firstSplit[0], "/")
	owner := secondSplit[0]
	repo := secondSplit[1]

	tagUrl := "https://api.github.com/repos/" + owner + "/" + repo + "/git/ref/tags/" + reference

	tagResponse, tagResponseErr := githubGet(tagUrl)

	if tagResponseErr != nil {
		return "", tagResponseErr
	}
	if tagResponse.StatusCode == http.StatusOK {
		defer tagResponse.Body.Close()
		var result GitRefResponse
		json.NewDecoder(tagResponse.Body).Decode(&result)
		if result.Object.Type == "tag" {
			annotatedUrl := "https://api.github.com/repos/" + owner + "/" + repo + "/git/tags/" + result.Object.SHA
			annotedResponse, annotatedResponseUrl := githubGet(annotatedUrl)
			if annotatedResponseUrl != nil {
				return "", annotatedResponseUrl
			}
			if annotedResponse.StatusCode == http.StatusOK {
				defer annotedResponse.Body.Close()
				var annotatedResult GitRefResponse
				json.NewDecoder(annotedResponse.Body).Decode(&annotatedResult)
				shaCache[value] = annotatedResult.Object.SHA
				return annotatedResult.Object.SHA, nil
			}

		}
		shaCache[value] = result.Object.SHA
		return result.Object.SHA, nil
	}
	defer tagResponse.Body.Close()
	headUrl := "https://api.github.com/repos/" + owner + "/" + repo + "/git/ref/heads/" + reference

	headReponse, headReponseErr := githubGet(headUrl)

	if headReponseErr != nil {
		return "", headReponseErr
	}
	if headReponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ref cannot be resolved (tag/branch not found)")
	}
	var result GitRefResponse
	defer headReponse.Body.Close()
	json.NewDecoder(headReponse.Body).Decode(&result)
	shaCache[value] = result.Object.SHA
	return result.Object.SHA, nil
}

func rewriteFile(path string, refs []ActionRef, resolved map[string]string) error {
	file, readErr := os.ReadFile(path)
	if readErr != nil {
		return readErr
	}
	lines := strings.Split(string(file), "\n")

	for _, line := range refs {
		sha := resolved[line.Value]
		if sha == "" {
			continue
		}
		original := line.Value
		replacement := original[:strings.Index(original, "@")] + "@" + sha + "  # pin: " + strings.Split(original, "@")[1]
		lines[line.Line-1] = strings.Replace(lines[line.Line-1], original, replacement, 1)
	}
	result := strings.Join(lines, "\n")

	return os.WriteFile(path, []byte(result), 0644)

}

var directory string
var dryRun bool
var output string
var skip string
var verify bool

// hardenCmd represents the harden command
var hardenCmd = &cobra.Command{
	Use:   "harden",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("harden called")
		if configData, err := os.ReadFile("forge.yml"); err == nil {
			var config ForgeConfig
			if yaml.Unmarshal(configData, &config) == nil {
				if !cmd.Flags().Changed("dir") && config.Harden.Dir != "" {
					directory = config.Harden.Dir
				}
				if !cmd.Flags().Changed("skip") && config.Harden.Skip != "" {
					skip = config.Harden.Skip
				}
			}
		}
		files, dirErr := directoryFind(directory)

		if dirErr != nil {
			fmt.Println(dirErr)
			os.Exit(2)
		}
		amount := 0
		for _, file := range files {
			content, contentErr := parseWorkflow(file)
			if contentErr != nil {
				fmt.Println(contentErr)
				continue
			}
			if verify {
				for _, ref := range content {
					if shouldSkip(ref.Value, skip) {
						continue
					}
					kind := category(ref)
					if kind == KindAction {
						amount += 1
						fmt.Printf("unpinned: %s (line %d)\n", ref.Value, ref.Line)
					}
				}
				continue
			}
			resolved := map[string]string{}
			for _, ref := range content {
				if shouldSkip(ref.Value, skip) {
					continue
				}
				kind := category(ref)
				if kind == KindAction {
					sha, shaErr := resolveRefToSHA(ref.Value)
					if shaErr != nil {
						fmt.Println("error:", shaErr)
						os.Exit(2)
					}
					resolved[ref.Value] = sha
					fmt.Printf("%s → %s\n", ref.Value, sha)
				} else if kind == KindReusable {
					fmt.Printf("warning: reusable workflow %s (line %d) cannot be auto-pinned\n", ref.Value, ref.Line)
				}
			}
			if !dryRun {
				rewriteFile(file, content, resolved)
			}
		}
		if verify {
			if amount > 0 {
				fmt.Printf("Currently you have %v dependencies unpinned \n", amount)
				os.Exit(1)
			}
			fmt.Println("all actions are pinned")
		}

	},
}

func init() {
	rootCmd.AddCommand(hardenCmd)
	hardenCmd.Flags().StringVar(&directory, "dir", ".github/workflows", "Directory to scan for workflow files")
	hardenCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show changes without writing them")
	hardenCmd.Flags().StringVar(&output, "output", "text", "Outputs formats are text, json, and diff")
	hardenCmd.Flags().StringVar(&skip, "skip", "", "Comma-separated list of actions or prefixes to skip (supports *glob, e.g. actions/*")
	hardenCmd.Flags().BoolVar(&verify, "verify", false, "Check if workflows are already fully pinned; exit 1 if not")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// hardenCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// hardenCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
