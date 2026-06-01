// Package main is the entry point for the Tiny TUI (Terminal User Interface)
// client. It connects to the URL and Auth gRPC services, initializes the
// Bubble Tea program with an alternate-screen buffer, and hands control to
// the interactive model. The TUI provides a keyboard-driven interface for
// creating short URLs, browsing existing links, and viewing click analytics
// without leaving the terminal.
package main

import (
	"fmt"
	"os"

	"github.com/Varun5711/shorternit/cmd/tui/client"
	"github.com/Varun5711/shorternit/cmd/tui/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Establish blocking gRPC connections to both backend services.
	// The TUI requires both to be reachable before it can render any
	// authenticated view, so we fail fast here rather than showing a
	// broken UI.
	grpcClient, err := client.NewClient("localhost:50051")
	if err != nil {
		fmt.Printf("Failed to connect to URL service: %v\n", err)
		os.Exit(1)
	}
	defer grpcClient.Close()

	authClient, err := client.NewAuthClient("localhost:50052")
	if err != nil {
		fmt.Printf("Failed to connect to auth service: %v\n", err)
		os.Exit(1)
	}
	defer authClient.Close()

	// WithAltScreen switches the terminal to the alternate buffer so
	// the user's scrollback is preserved when the TUI exits.
	p := tea.NewProgram(
		ui.NewModel(grpcClient, authClient),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
