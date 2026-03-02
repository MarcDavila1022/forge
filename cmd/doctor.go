/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"os/exec"

	"github.com/spf13/cobra"

	"bytes"
)

// doctorCmd represents the doctor command
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Verifies whether an app is trustable",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:.`,
	Run: func(cmd *cobra.Command, args []string) {
		_, ghErr := exec.Command("gh", "--version").CombinedOutput()
		_, insideTreeErr := exec.Command("git", "rev-parse", "--is-inside-work-tree").CombinedOutput()
		remoteOut, remoteErr := exec.Command("git", "remote").CombinedOutput()
		_, authErr := exec.Command("gh", "auth", "status").CombinedOutput()
		if ghErr != nil {
			fmt.Printf("\u274C gh not found - please install it: https://cli.github.com \n")
			fmt.Printf("\u26A0\uFE0F gh auth - skipped (gh not installed)")
		} else {
			fmt.Printf("\u2705 gh is installed\n")
			if authErr != nil {
				fmt.Printf("\u274C login not found -  run: gh auth login\n")
			} else {
				fmt.Printf("\u2705 logged into GitHub\n")
			}
		}

		if remoteErr != nil {
			fmt.Printf("\u274C git command not found, is git installed? - run: which git  \n")
			fmt.Printf("\u26A0\uFE0F git remote - skipped (git not installed)")
		} else if insideTreeErr != nil {
			fmt.Printf("\u274C repo not found- run: git init\n")
			fmt.Printf("\u26A0\uFE0F git remote - skipped (no repo to check)")
		} else {
			fmt.Printf("\u2705 inside a git repository\n")
			if len(bytes.TrimSpace(remoteOut)) == 0 {
				fmt.Printf("\u274C git remote not found - git remote add origin <REMOTE_URL> \n")
			} else {
				urlOut, _ := exec.Command("git", "remote", "get-url", "origin").CombinedOutput()
				if !(bytes.Contains(urlOut, []byte("github.com"))) {
					fmt.Printf("\u274C remote is not in github\n")
				} else {

					fmt.Printf("\u2705 git remote exists\n")
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// doctorCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// doctorCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
