package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type model struct {
	client    *openai.Client
	modelName string
	messages  []chatMessage
	input     string
	viewport  string
	loading   bool
	err       error
}

type chatMessage struct {
	role    string
	content string
}

type msgResponse struct {
	content string
	err     error
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED")).
			MarginBottom(1)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981")).
			Bold(true)

	assistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3B82F6")).
			Bold(true)

	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Italic(true)
)

func initialModel() model {
	godotenv.Load()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return model{err: fmt.Errorf("OPENAI_API_KEY not found in environment or .env file")}
	}

	modelName := os.Getenv("OPENAI_MODEL")
	if modelName == "" {
		modelName = "gpt-4o"
	}

	client := openai.NewClient(option.WithAPIKey(apiKey))

	return model{
		client:    &client,
		modelName: modelName,
		messages:  []chatMessage{},
		input:     "",
		viewport:  "",
		loading:   false,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if m.input != "" && !m.loading {
				userMsg := chatMessage{role: "user", content: m.input}
				m.messages = append(m.messages, userMsg)
				m.viewport += userStyle.Render("You: ") + m.input + "\n\n"

				m.input = ""
				m.loading = true

				return m, m.sendMessage()
			}
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		default:
			if !m.loading {
				m.input += msg.String()
			}
		}
	case msgResponse:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			assistantMsg := chatMessage{role: "assistant", content: msg.content}
			m.messages = append(m.messages, assistantMsg)
			m.viewport += assistantStyle.Render("LLM: ") + msg.content + "\n\n"
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err)) + "\n\n" +
			helpStyle.Render("Press q to quit.")
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("LLM TUI Chat"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("================"))
	b.WriteString("\n\n")

	b.WriteString(m.viewport)

	if m.loading {
		b.WriteString(assistantStyle.Render("LLM is typing..."))
		b.WriteString("\n\n")
	}

	b.WriteString(inputStyle.Render("You: ") + m.input)
	if !m.loading {
		b.WriteString(inputStyle.Render("█"))
	}
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("Press Enter to send, Ctrl+C or q to quit"))

	return b.String()
}

func (m model) sendMessage() tea.Cmd {
	return func() tea.Msg {
		messages := make([]openai.ChatCompletionMessageParamUnion, len(m.messages))
		for i, msg := range m.messages {
			if msg.role == "user" {
				messages[i] = openai.UserMessage(msg.content)
			} else {
				messages[i] = openai.AssistantMessage(msg.content)
			}
		}

		resp, err := m.client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
			Messages: messages,
			Model:    openai.ChatModel(m.modelName),
		})
		if err != nil {
			return msgResponse{err: err}
		}

		if len(resp.Choices) == 0 {
			return msgResponse{err: fmt.Errorf("no response from OpenAI")}
		}

		return msgResponse{content: resp.Choices[0].Message.Content}
	}
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}
