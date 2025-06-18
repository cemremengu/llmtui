package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type model struct {
	client      *openai.Client
	modelName   string
	messages    []chatMessage
	input       string
	viewport    string
	loading     bool
	streaming   bool
	partialResp string
	err         error
	streamChan  chan string
}

type chatMessage struct {
	role    string
	content string
}

type msgResponse struct {
	content string
	err     error
}

type msgStreamChunk struct {
	chunk string
	done  bool
	err   error
}

type (
	streamStartMsg  struct{}
	streamUpdateMsg struct {
		content string
	}
)
type streamCompleteMsg struct {
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
	case streamStartMsg:
		m.streaming = true
		m.partialResp = ""
	case streamStarted:
		// Start streaming with a new subscription
		m.streamChan = make(chan string, 100)
		go startStreamingInBackground(m.streamChan, msg.client, msg.messages, msg.modelName)
		return m, listenForStreamUpdates(m.streamChan)
	case streamUpdateMsg:
		if msg.content != "" {
			m.partialResp = msg.content
		}
		// Continue listening for updates using a stored channel
		if m.streamChan != nil {
			return m, listenForStreamUpdates(m.streamChan)
		}
		return m, nil
	case streamCompleteMsg:
		m.loading = false
		m.streaming = false
		m.streamChan = nil
		if msg.err != nil {
			m.err = msg.err
		} else {
			assistantMsg := chatMessage{role: "assistant", content: msg.content}
			m.messages = append(m.messages, assistantMsg)
			m.viewport += assistantStyle.Render("LLM: ") + msg.content + "\n\n"
		}
		m.partialResp = ""
	case msgStreamChunk:
		if msg.err != nil {
			m.err = msg.err
			m.loading = false
			m.streaming = false
			m.partialResp = ""
		} else if msg.done {
			m.loading = false
			m.streaming = false
			assistantMsg := chatMessage{role: "assistant", content: msg.chunk}
			m.messages = append(m.messages, assistantMsg)
			m.viewport += assistantStyle.Render("LLM: ") + msg.chunk + "\n\n"
			m.partialResp = ""
		} else {
			m.partialResp = msg.chunk
			m.streaming = true
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
		if m.streaming && m.partialResp != "" {
			b.WriteString(assistantStyle.Render("LLM: ") + m.partialResp + assistantStyle.Render("█"))
		} else {
			b.WriteString(assistantStyle.Render("LLM is typing..."))
		}
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
	return tea.Batch(
		func() tea.Msg { return streamStartMsg{} },
		m.streamResponse(),
	)
}

func (m model) streamResponse() tea.Cmd {
	return func() tea.Msg {
		messages := make([]openai.ChatCompletionMessageParamUnion, len(m.messages))
		for i, msg := range m.messages {
			if msg.role == "user" {
				messages[i] = openai.UserMessage(msg.content)
			} else {
				messages[i] = openai.AssistantMessage(msg.content)
			}
		}

		// Start streaming and return the subscription
		return streamStarted{
			client:    m.client,
			messages:  messages,
			modelName: m.modelName,
		}
	}
}

type streamStarted struct {
	client    *openai.Client
	messages  []openai.ChatCompletionMessageParamUnion
	modelName string
}

func startStreamingInBackground(streamChan chan string, client *openai.Client, messages []openai.ChatCompletionMessageParamUnion, modelName string) {
	defer close(streamChan)
	
	ctx := context.Background()
	stream := client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    openai.ChatModel(modelName),
	})

	var fullResponse strings.Builder
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			fullResponse.WriteString(chunk.Choices[0].Delta.Content)
			// Send accumulated content to channel
			select {
			case streamChan <- fullResponse.String():
			default:
			}
		}
	}
	
	// Send final result
	if stream.Err() == nil {
		select {
		case streamChan <- "DONE:" + fullResponse.String():
		default:
		}
	} else {
		select {
		case streamChan <- "ERROR:" + stream.Err().Error():
		default:
		}
	}
}

func listenForStreamUpdates(streamChan <-chan string) tea.Cmd {
	return func() tea.Msg {
		select {
		case content, ok := <-streamChan:
			if !ok {
				// Channel closed - streaming is done
				return streamCompleteMsg{content: "", err: nil}
			}
			if strings.HasPrefix(content, "DONE:") {
				return streamCompleteMsg{content: content[5:], err: nil}
			}
			if strings.HasPrefix(content, "ERROR:") {
				return streamCompleteMsg{content: "", err: fmt.Errorf("%s", content[6:])}
			}
			return streamUpdateMsg{content: content}
		case <-time.After(50 * time.Millisecond):
			// No update yet, return empty update and continue listening
			return streamUpdateMsg{}
		}
	}
}



func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}
