package selfupdate_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/maccavelli/mcplib/selfupdate"
)

func ExampleNew_standalone() {
	src, err := selfupdate.NewGitHubSource(selfupdate.GitHubOptions{
		Repository: selfupdate.Repository{Owner: "maccavelli", Name: "prepare-commit-msg"},
		Client:     &http.Client{Timeout: 15 * time.Minute},
		UserAgent:  "prepare-commit-msg/v1.2.0",
		Limits:     selfupdate.DefaultLimits(),
	})
	if err != nil {
		fmt.Println("source:", err)
		return
	}
	selector, err := selfupdate.NewExactAssetSelector([]selfupdate.Platform{
		{OS: "linux", Arch: "amd64"},
		{OS: "darwin", Arch: "arm64"},
		{OS: "windows", Arch: "amd64"},
	})
	if err != nil {
		fmt.Println("selector:", err)
		return
	}
	installer, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{})
	if err != nil {
		fmt.Println("installer:", err)
		return
	}
	updater, err := selfupdate.New(selfupdate.Config{
		Source:    src,
		Versions:  selfupdate.NewStrictVersionPolicy(),
		Assets:    selector,
		Installer: installer,
		Reporter:  selfupdate.NewTextReporter(os.Stderr),
		Confirmer: selfupdate.NewTerminalConfirmer(os.Stdin, os.Stderr),
		Limits:    selfupdate.DefaultLimits(),
	})
	if err != nil {
		fmt.Println("new:", err)
		return
	}
	_ = updater
	fmt.Println("standalone updater ready")
	// Output: standalone updater ready
}

func ExampleNewTextReporter() {
	reporter := selfupdate.NewTextReporter(os.Stdout)
	_ = reporter.Report(context.Background(), selfupdate.Event{
		Kind:    selfupdate.EventSelected,
		Product: "demo",
		Target:  "v1.1.0",
		Asset:   "demo-linux-amd64",
	})
	// Output: selfupdate: selected product=demo target=v1.1.0 asset=demo-linux-amd64
}

func ExampleNewManagedInstaller() {
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("compose managed installer around %T\n", inner)
	// Output: compose managed installer around *selfupdate.StandaloneInstaller
}
