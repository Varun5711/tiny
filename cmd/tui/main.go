package main

import (
	"fmt"
	"os"

	"github.com/Varun5711/shorternit/cmd/tui/client"
	"github.com/Varun5711/shorternit/cmd/tui/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {

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

	p := tea.NewProgram(
		ui.NewModel(grpcClient, authClient),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
