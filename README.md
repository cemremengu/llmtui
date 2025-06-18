# ChatGPT TUI

A simple terminal user interface for chatting with ChatGPT using the OpenAI API.

## Setup

1. Set your OpenAI API key either:
   - As an environment variable:
     ```bash
     export OPENAI_API_KEY="your-api-key-here"
     ```
   - Or create a `.env` file:
     ```bash
     cp .env.example .env
     # Edit .env and add your API key
     ```

2. Run the application:
   ```bash
   go run main.go
   ```

## Usage

- Type your message and press Enter to send
- Press Ctrl+C or 'q' to quit
- The app uses GPT-4o model by default

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- [OpenAI Go SDK](https://github.com/openai/openai-go) - OpenAI API client