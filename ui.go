package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
	"os"
	"path/filepath"
	"bytes"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strconv"
    "unicode"


	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
  	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/api/gmail/v1"
)

type state int

const (
	inbox state = iota
	viewing
	loading
	composing
	replying
	searching
	managingLabels
)

type keyMap struct {
	Back           key.Binding
	Reply          key.Binding
	Compose        key.Binding
	Delete         key.Binding
	Search         key.Binding
	Labels         key.Binding
	ToggleRead     key.Binding
	Quit           key.Binding
	Send           key.Binding
	NextInput      key.Binding
	PrevInput      key.Binding
	ShowHelp       key.Binding
	CloseHelp      key.Binding
	Select         key.Binding
	AddAttachment  key.Binding
	RemoveAttachment key.Binding
	DownloadAttachment key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.ShowHelp, k.Compose, k.Search, k.Labels, k.Quit,
	}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Compose, k.Reply, k.Search, k.Labels},
		{k.Delete, k.ToggleRead, k.Back, k.Quit},
		{k.Send, k.NextInput, k.PrevInput},
		{k.ShowHelp, k.CloseHelp, k.Select, k.AddAttachment, k.RemoveAttachment},
	}
}

var keys = keyMap{
	Back: key.NewBinding(
		key.WithKeys("b", "esc"),
		key.WithHelp("b/esc", "back"),
	),
	Reply: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "reply"),
	),
	Compose: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "compose"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Labels: key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("l", "labels"),
	),
	ToggleRead: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "mark read/unread"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Send: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "send"),
	),
	NextInput: key.NewBinding(
        key.WithKeys("tab"),
        key.WithHelp("tab", "next field"),
    ),
    PrevInput: key.NewBinding(
        key.WithKeys("shift+tab"),
        key.WithHelp("shift+tab", "prev field"),
    ),
	ShowHelp: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	CloseHelp: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "close help"),
	),
	Select: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	AddAttachment: key.NewBinding(
    key.WithKeys("ctrl+a"),
    key.WithHelp("ctrl+a", "add attachment"),
	),
	RemoveAttachment: key.NewBinding(
    key.WithKeys("ctrl+x"),
    key.WithHelp("ctrl+x", "remove attachment"),
	),
	DownloadAttachment: key.NewBinding(
    key.WithKeys("a"),
    key.WithHelp("a", "download attachment"),
	),
}


type emailItem struct {
	id          string
	threadId    string
	subject     string
	from        string
	snippet     string
	date        string
	labels      []string
	isUnread    bool
	body        string
	recipient   string
	cc          string
	bcc         string
	attachments []*gmail.MessagePart
}

func (e emailItem) Title() string {
	if e.isUnread {
		return "● " + e.subject
	}
	return "  " + e.subject
}

func (e emailItem) Description() string {
	return fmt.Sprintf("%s - %s", e.from, e.snippet)
}

func (e emailItem) FilterValue() string { return e.subject + " " + e.from }

type labelItem struct {
	label *gmail.Label
}

func (l labelItem) Title() string       { return l.label.Name }
func (l labelItem) Description() string { return fmt.Sprintf("ID: %s", l.label.Id) }
func (l labelItem) FilterValue() string { return l.label.Name }

type model struct {
	state             state
	list              list.Model
	srv               *gmail.Service
	fullEmail         string
	loading           spinner.Model
	viewport          viewport.Model
	width             int
	height            int
	err               string
	help              help.Model
	showHelp          bool
	composeFrom       textinput.Model
	composeTo         textinput.Model
	composeCc         textinput.Model
	composeBcc        textinput.Model
	composeSubj       textinput.Model
	composeBody       textarea.Model
	replyBody         textarea.Model
	searchInput       textinput.Model
	labels            []*gmail.Label
	labelsList        list.Model
	currentMsg        *emailItem
	replyToMsg        *emailItem
	focused           int
	searchQuery       string
	composeAttachments []string
	replyAttachments   []string
	attachmentInput    textinput.Model
	addingAttachment   bool
	composeFocus int
	attachmentDownloading bool
	downloadingIndex      int

}

func initialModel(emails []*gmail.Message, srv *gmail.Service, labels []*gmail.Label) model {
	items := []list.Item{}
	for _, msg := range emails {
		item := createEmailItem(srv, msg.Id, false)
		if item != nil {
			items = append(items, *item)
		}
	}

	composeBody := textarea.New()
	composeBody.Placeholder = "Compose your message here..."
	composeBody.Focus()
	composeBody.CharLimit = 0
	composeBody.SetWidth(80)
	composeBody.SetHeight(10)

	replyBody := textarea.New()
	replyBody.Placeholder = "Type your reply here..."
	replyBody.CharLimit = 0
	replyBody.SetWidth(80)
	replyBody.SetHeight(10)

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		BorderForeground(lipgloss.Color("62")).
		Foreground(lipgloss.Color("62"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedTitle.Copy().
		Foreground(lipgloss.Color("245"))

	l := list.New(items, delegate, 0, 0)
	l.Title = "Inbox"
	l.Styles.Title = lipgloss.NewStyle().MarginLeft(2)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	l.KeyMap.Quit = key.NewBinding(key.WithKeys("q"))
	l.SetSize(0, 0)

	labelsList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	labelsList.Title = "Labels"
	labelsList.SetShowHelp(false)
	labelsList.DisableQuitKeybindings()
	labelsList.KeyMap.Quit = key.NewBinding(key.WithKeys("q"))
	labelsList.KeyMap.CursorUp = key.NewBinding(key.WithKeys("up", "k"))
	labelsList.KeyMap.CursorDown = key.NewBinding(key.WithKeys("down", "j"))
	labelsList.KeyMap.GoToStart = key.NewBinding(key.WithKeys("home", "g"))
	labelsList.KeyMap.GoToEnd = key.NewBinding(key.WithKeys("end", "G"))

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	vp := viewport.New(20, 10)
	vp.Style = lipgloss.NewStyle().Padding(0, 1)

	from := textinput.New()
    from.Placeholder = "From"
    from.Focus()
    from.CharLimit = 100

	to := textinput.New()
    to.Placeholder = "To"
    to.CharLimit = 100

    cc := textinput.New()
    cc.Placeholder = "CC"
    cc.CharLimit = 100


    bcc := textinput.New()
    bcc.Placeholder = "BCC"
    bcc.CharLimit = 100

    subj := textinput.New()
    subj.Placeholder = "Subject"
    subj.CharLimit = 200

	search := textinput.New()
	search.Placeholder = "Search emails..."

	attachmentInput := textinput.New()
	attachmentInput.Placeholder = "Path to attachment..."

	help := help.New()
	help.ShowAll = false

	return model{
		state:             inbox,
		list:              l,
		srv:               srv,
		loading:           s,
		viewport:          vp,
		help:              help,
		composeFrom:       from,
		composeTo:         to,
		composeCc:         cc,
		composeBcc:        bcc,
		composeSubj:       subj,
		composeBody:       composeBody,
		replyBody:         replyBody,
		searchInput:       search,
		labels:            labels,
		labelsList:        labelsList,
		attachmentInput:   attachmentInput,
		composeAttachments: []string{},
		replyAttachments:   []string{},
	}
}

func (m model) Init() tea.Cmd {
	return m.loading.Tick
}

func loadEmailsByLabel(srv *gmail.Service, labelID string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := srv.Users.Messages.List("me").LabelIds(labelID).MaxResults(10).Do()
		if err != nil {
			return emailLoadErrorMsg{err: err}
		}
		return searchResultMsg{messages: msgs.Messages}
	}
}


func downloadAttachment(srv *gmail.Service, msgID string, attachment *gmail.MessagePart) tea.Cmd {
    return func() tea.Msg {
        att, err := srv.Users.Messages.Attachments.Get("me", msgID, attachment.Body.AttachmentId).Do()
        if err != nil {
            return notificationMsg{message: fmt.Sprintf("Download failed: %v", err)}
        }

        data, err := base64.URLEncoding.DecodeString(att.Data)
        if err != nil {
            data, err = base64.StdEncoding.DecodeString(att.Data)
            if err != nil {
                return notificationMsg{message: "Failed to decode attachment"}
            }
        }

        filename := sanitizeFilename(attachment.Filename)
        if err := os.WriteFile(filename, data, 0644); err != nil {
            return notificationMsg{message: fmt.Sprintf("Save failed: %v", err)}
        }

        return attachmentDownloadedMsg{filename: filename}
    }
}

func sanitizeFilename(name string) string {
    return strings.Map(func(r rune) rune {
        if unicode.IsSpace(r) || unicode.IsLetter(r) || unicode.IsNumber(r) || 
           r == '-' || r == '_' || r == '.' {
            return r
        }
        return '_'
    }, name)
}



func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.help.Width = msg.Width

        if m.state == inbox {
            m.list.SetSize(msg.Width, msg.Height-3)
        } else if m.state == viewing {
            m.viewport.Width = msg.Width
            m.viewport.Height = msg.Height - 7
        }
        return m, nil

    case tea.KeyMsg:
        if !m.showHelp {
            switch {
            case key.Matches(msg, keys.ShowHelp):
                m.showHelp = true
                return m, nil
            }
        } else {
            switch {
            case key.Matches(msg, keys.CloseHelp):
                m.showHelp = false
                return m, nil
            }
        }

        if m.state == viewing && m.attachmentDownloading {
            switch {
            case key.Matches(msg, keys.Back):
                m.attachmentDownloading = false
                return m, nil
            case msg.Type == tea.KeyRunes:
    			if digit, err := strconv.Atoi(string(msg.Runes)); err == nil {
    			    if digit > 0 && digit <= len(m.currentMsg.attachments) {
    			        return m, downloadAttachment(
    			            m.srv, 
    			            m.currentMsg.id, 
    			            m.currentMsg.attachments[digit-1],
    			        )
    			    }
    			}
            }
        }

        switch m.state {
        case inbox:
            return updateInbox(msg, m)
        case viewing:
            return updateViewing(msg, m)
        case composing:
            return updateComposing(msg, m)
        case replying:
            return updateReplying(msg, m)
        case searching:
            return updateSearching(msg, m)
        case managingLabels:
            return updateLabelManagement(msg, m)
        }

    case emailLoadedMsg:
        m.state = viewing
        m.fullEmail = msg.content
        m.viewport.Width = m.width
        m.viewport.Height = m.height - 7
        m.viewport.SetContent(m.fullEmail)
        return m, nil

    case emailSentMsg:
        m.state = inbox
        m.viewport.GotoTop()
        return m, tea.Batch(showNotification("Email sent successfully!"))

    case labelsLoadedMsg:
        items := make([]list.Item, len(msg.labels))
        for i, label := range msg.labels {
            items[i] = labelItem{label: label}
        }
        m.labels = msg.labels
        m.labelsList.SetItems(items)
        m.state = managingLabels
        return m, nil

    case searchResultMsg:
        items := []list.Item{}
        for _, msg := range msg.messages {
            item := createEmailItem(m.srv, msg.Id, true)
            if item != nil {
                items = append(items, *item)
            }
        }
        m.list.SetItems(items)
        m.state = inbox
        return m, nil

    case attachmentDownloadedMsg:
        return m, showNotification(fmt.Sprintf("Downloaded: %s", msg.filename))
    }

    // Handle other states
    switch m.state {
    case loading:
        var cmd tea.Cmd
        m.loading, cmd = m.loading.Update(msg)
        cmds = append(cmds, cmd)
    case viewing:
        var cmd tea.Cmd
        m.viewport, cmd = m.viewport.Update(msg)
        cmds = append(cmds, cmd)
    case replying:
        var cmd tea.Cmd
        m.replyBody, cmd = m.replyBody.Update(msg)
        cmds = append(cmds, cmd)
    case searching:
        var cmd tea.Cmd
        m.searchInput, cmd = m.searchInput.Update(msg)
        cmds = append(cmds, cmd)
    case managingLabels:
        var cmd tea.Cmd
        m.labelsList, cmd = m.labelsList.Update(msg)
        cmds = append(cmds, cmd)
    }

    return m, tea.Batch(cmds...)
}


func (m model) View() string {
	if m.showHelp {
		return m.help.View(keys)
	}

	switch m.state {
	case inbox:
		return inboxView(m)
	case viewing:
		return emailView(m)
	case loading:
		return loadingView(m)
	case composing:
		return composeView(m)
	case replying:
		return replyView(m)
	case searching:
		return searchView(m)
	case managingLabels:
		return labelsView(m)
	default:
		return ""
	}
}

func findAttachments(part *gmail.MessagePart) []*gmail.MessagePart {
    var attachments []*gmail.MessagePart
    if strings.HasPrefix(part.MimeType, "multipart/") {
        for _, p := range part.Parts {
            attachments = append(attachments, findAttachments(p)...)
        }
    } else if part.Filename != "" {
        attachments = append(attachments, part)
    }
    return attachments
}

func createEmailItem(srv *gmail.Service, msgId string, minimal bool) *emailItem {
	if srv == nil {
		log.Println("Gmail service is not initialized")
		return nil
	}

	var msg *gmail.Message
	var err error

	if minimal {
		msg, err = srv.Users.Messages.Get("me", msgId).Format("minimal").Do()
	} else {
		msg, err = srv.Users.Messages.Get("me", msgId).Format("full").Do()
	}

	if err != nil {
		log.Printf("Error fetching message %s: %v\n", msgId, err)
		return nil
	}

	if msg == nil {
		log.Printf("Received nil message for ID %s\n", msgId)
		return nil
	}

	item := &emailItem{
		id:       msg.Id,
		threadId: msg.ThreadId,
		snippet:  msg.Snippet,
	}

	if msg.Payload != nil {
    for _, h := range msg.Payload.Headers {
        switch h.Name {
        case "Subject":
            item.subject = h.Value
        case "From":
            item.from = h.Value
        case "Date":
            item.date = formatDate(h.Value)
        case "To":
            item.recipient = h.Value
        case "Cc":
            item.cc = h.Value
        case "Bcc":
            item.bcc = h.Value
        }
    }

    if !minimal {
        item.body = extractPlainText(msg.Payload)
        item.attachments = findAttachments(msg.Payload)
    }
}

	for _, labelId := range msg.LabelIds {
		if labelId == "UNREAD" {
			item.isUnread = true
		}
		item.labels = append(item.labels, labelId)
	}

	if len(item.snippet) > 80 {
		item.snippet = item.snippet[:77] + "..."
	}

	return item
}


func inboxView(m model) string {
	help := "\n[c] compose • [r] reply • [d] delete • [m] mark read/unread • [l] labels • [/] search • [?] help • [q] quit\n"
	return m.list.View() + help
}


func emailView(m model) string {
	b := strings.Builder{}
	b.WriteString(fmt.Sprintf("\nFrom: %s\n", m.currentMsg.from))
	b.WriteString(fmt.Sprintf("To: %s\n", m.currentMsg.recipient))
	if m.currentMsg.cc != "" {
		b.WriteString(fmt.Sprintf("CC: %s\n", m.currentMsg.cc))
	}
	if m.currentMsg.bcc != "" {
		b.WriteString(fmt.Sprintf("BCC: %s\n", m.currentMsg.bcc))
	}
	if m.attachmentDownloading {
    	b.WriteString("\n\nDownload which attachment? (1-9) [esc] cancel")
	}
	b.WriteString(fmt.Sprintf("Subject: %s\n", m.currentMsg.subject))
	b.WriteString(fmt.Sprintf("Date: %s\n\n", m.currentMsg.date))
	b.WriteString(m.viewport.View())

	if len(m.currentMsg.attachments) > 0 {
		b.WriteString("\n\nAttachments:\n")
		for i, att := range m.currentMsg.attachments {
			b.WriteString(fmt.Sprintf("  [%d] %s (%s)\n", i+1, att.Filename, humanSize(att.Body.Size)))
		}
	}

	b.WriteString("\n\n[b] back • [r] reply • [d] delete • [m] mark read/unread • [a] download attachment • [q] quit\n")
	return b.String()
}

func loadingView(m model) string {
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(
			lipgloss.Center,
			m.loading.View(),
			"Loading...",
		),
	)
}

func composeView(m model) string {
	view := strings.Builder{}
	view.WriteString("\n  Compose New Email\n\n")
	view.WriteString(fmt.Sprintf("  From: %s\n", m.composeFrom.View()))
	view.WriteString(fmt.Sprintf("  To:   %s\n", m.composeTo.View()))
	view.WriteString(fmt.Sprintf("  CC:   %s\n", m.composeCc.View()))
	view.WriteString(fmt.Sprintf("  BCC:  %s\n", m.composeBcc.View()))
	view.WriteString(fmt.Sprintf("  Subj: %s\n\n", m.composeSubj.View()))
	view.WriteString("  Body:\n" + m.composeBody.View() + "\n")

	if len(m.composeAttachments) > 0 {
		view.WriteString("\nAttachments:\n")
		for i, f := range m.composeAttachments {
			view.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, filepath.Base(f)))
		}
	}

	if m.addingAttachment {
		view.WriteString("\nAttachment Path: " + m.attachmentInput.View())
	}

	view.WriteString("\n[ctrl+s] send • [ctrl+a] add attachment • [ctrl+x] remove attachment • [esc] back")

	return view.String()
}

func replyView(m model) string {
	view := strings.Builder{}
	view.WriteString(fmt.Sprintf("\n  Reply to: %s\n", m.replyToMsg.from))
	view.WriteString(fmt.Sprintf("  Subject: Re: %s\n\n", m.replyToMsg.subject))
	view.WriteString(m.replyBody.View() + "\n")

	if len(m.replyAttachments) > 0 {
		view.WriteString("\nAttachments:\n")
		for i, f := range m.replyAttachments {
			view.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, filepath.Base(f)))
		}
	}

	if m.addingAttachment {
		view.WriteString("\nAttachment Path: " + m.attachmentInput.View())
	}

	view.WriteString("\n[ctrl+s] send • [ctrl+a] add attachment • [ctrl+x] remove attachment • [esc] back")
    return view.String()

}


func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func searchView(m model) string {
	return "\n  Search: " + m.searchInput.View() + "\n\n[enter] search • [esc] cancel\n"
}

func labelsView(m model) string {
	help := "\n[↑/↓] navigate • [enter] select • [b] back\n"
	return m.labelsList.View() + help
}

func updateInbox(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch {
        case key.Matches(msg, keys.Compose):
            m.state = composing
            m.composeFrom.SetValue("me")
            return m, nil

        case key.Matches(msg, keys.Search):
            m.state = searching
            m.searchInput.Focus()
            return m, nil

        case key.Matches(msg, keys.Labels):
            return m, loadLabels(m.srv)

        case key.Matches(msg, keys.Quit):
            return m, tea.Quit

        case msg.String() == "enter":
            selected, ok := m.list.SelectedItem().(emailItem)
            if !ok {
                return m, nil
            }
            m.currentMsg = &selected
            m.state = loading
            return m, tea.Batch(
                m.loading.Tick,
                loadEmail(m.srv, selected.id),
            )

        case key.Matches(msg, keys.Delete):
            selected, ok := m.list.SelectedItem().(emailItem)
            if ok {
                return m, deleteEmail(m.srv, selected.id)
            }

        case key.Matches(msg, keys.ToggleRead):
            selected, ok := m.list.SelectedItem().(emailItem)
            if ok {
                return m, toggleReadStatus(m.srv, selected.id, selected.isUnread)
            }
        }
    }

    m.list, cmd = m.list.Update(msg)
    return m, cmd
}

type emailLoadErrorMsg struct {
    err error	
}
type attachmentDownloadedMsg struct{
	filename string
}


func updateViewing(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch {
        case key.Matches(msg, keys.Back):
            m.state = inbox
            m.viewport.GotoTop()
            return m, nil

        case key.Matches(msg, keys.Reply):
            m.state = replying
            m.replyToMsg = m.currentMsg
            m.replyBody.Focus()
            return m, nil

        case key.Matches(msg, keys.Delete):
            return m, deleteEmail(m.srv, m.currentMsg.id)

        case key.Matches(msg, keys.ToggleRead):
            return m, toggleReadStatus(m.srv, m.currentMsg.id, m.currentMsg.isUnread)

        case key.Matches(msg, keys.Labels):
            return m, loadLabels(m.srv)

        case key.Matches(msg, keys.Quit):
            return m, tea.Quit
        }
		case key.Matches(msg, keys.DownloadAttachment):
    	    if len(m.currentMsg.attachments) == 0 {
    	        return m, showNotification("No attachments available")
    	    }
    	    m.attachmentDownloading = true
    	    m.downloadingIndex = 0
    	    return m, showNotification("Select attachment to download (1-9)")
    	}
    }

    m.viewport, cmd = m.viewport.Update(msg)
    return m, cmd
}

func (m *model) handleTabNavigation(msg tea.KeyMsg) tea.Cmd {
    if m.focused == 3 && m.composeBody.Focused() {
        var cmd tea.Cmd
        m.composeBody, cmd = m.composeBody.Update(msg)
        return cmd
    }
    
    if msg.String() == "tab" {
        m.focused = (m.focused + 1) % 4
    } else {
        m.focused = (m.focused - 1 + 4) % 4
    }
    return m.focusField()
}

func (m *model) updateFocusedField(msg tea.Msg) tea.Cmd {
    var cmd tea.Cmd
    switch m.focused {
    case 0:
        m.composeFrom, cmd = m.composeFrom.Update(msg)
    case 1:
        m.composeTo, cmd = m.composeTo.Update(msg)
    case 2:
        m.composeSubj, cmd = m.composeSubj.Update(msg)
    case 3:
        m.composeBody, cmd = m.composeBody.Update(msg)
    }
    return cmd
}

func updateComposing(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd

    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch {
        case key.Matches(msg, keys.Back):
            if m.addingAttachment {
                // Cancel attachment input
                m.addingAttachment = false
                m.attachmentInput.Reset()
                return m, m.focusComposeField()
            } else {
                m.state = inbox
                return m, nil
            }

        case key.Matches(msg, keys.Send):
            return m, sendEmail(
                m.srv,
                m.composeTo.Value(),
                m.composeCc.Value(),
                m.composeBcc.Value(),
                m.composeSubj.Value(),
                m.composeBody.Value(),
                m.composeAttachments,
            )

        case key.Matches(msg, keys.AddAttachment):
            if !m.addingAttachment {
                m.addingAttachment = true
                m.attachmentInput.Focus()
                return m, nil
            }

        case key.Matches(msg, keys.RemoveAttachment):
            if !m.addingAttachment && len(m.composeAttachments) > 0 {
                m.composeAttachments = m.composeAttachments[:len(m.composeAttachments)-1]
                return m, nil
            }

        case msg.Type == tea.KeyEnter && m.addingAttachment:
            path := m.attachmentInput.Value()
            if _, err := os.Stat(path); err == nil {
                m.composeAttachments = append(m.composeAttachments, path)
                m.addingAttachment = false
                m.attachmentInput.Reset()
                // Return focus to the body after adding attachment
                return m, m.focusComposeField()
            }
            return m, nil

        case key.Matches(msg, keys.NextInput):
            if !m.addingAttachment {
                m.focused = (m.focused + 1) % 6
                return m, m.focusComposeField()
            }

        case key.Matches(msg, keys.PrevInput):
            if !m.addingAttachment {
                m.focused = (m.focused - 1 + 6) % 6
                return m, m.focusComposeField()
            }
        }
    }

    if m.addingAttachment {
        m.attachmentInput, cmd = m.attachmentInput.Update(msg)
    } else {
        switch m.focused {
        case 0:
            m.composeFrom, cmd = m.composeFrom.Update(msg)
        case 1:
            m.composeTo, cmd = m.composeTo.Update(msg)
        case 2:
            m.composeCc, cmd = m.composeCc.Update(msg)
        case 3:
            m.composeBcc, cmd = m.composeBcc.Update(msg)
        case 4:
            m.composeSubj, cmd = m.composeSubj.Update(msg)
        case 5:
            m.composeBody, cmd = m.composeBody.Update(msg)
        }
    }
    return m, cmd
}

func handleComposeInput(msg tea.KeyMsg, m model) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    switch m.focused {
    case 0:
        m.composeFrom, cmd = m.composeFrom.Update(msg)
    case 1:
        m.composeTo, cmd = m.composeTo.Update(msg)
    case 2:
        m.composeCc, cmd = m.composeCc.Update(msg)
    case 3:
        m.composeBcc, cmd = m.composeBcc.Update(msg)
    case 4:
        m.composeSubj, cmd = m.composeSubj.Update(msg)
    case 5:
        m.composeBody, cmd = m.composeBody.Update(msg)
    }
    return m, cmd
}


func (m *model) focusComposeField() tea.Cmd {
    m.composeFrom.Blur()
    m.composeTo.Blur()
    m.composeCc.Blur()
    m.composeBcc.Blur()
    m.composeSubj.Blur()
    m.composeBody.Blur()

    switch m.focused {
    case 0:
        return m.composeFrom.Focus()
    case 1:
        return m.composeTo.Focus()
    case 2:
        return m.composeCc.Focus()
    case 3:
        return m.composeBcc.Focus()
    case 4:
        return m.composeSubj.Focus()
    case 5:
        return m.composeBody.Focus()
    }
    return nil
}


func (m *model) focusField() tea.Cmd {
    m.composeFrom.Blur()
    m.composeTo.Blur()
    m.composeSubj.Blur()
    m.composeBody.Blur()

    switch m.focused {
    case 0:
        return m.composeFrom.Focus()
    case 1:
        return m.composeTo.Focus()
    case 2:
        return m.composeSubj.Focus()
    case 3:
        return m.composeBody.Focus()
    }
    return nil
}

func updateReplying(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			m.state = viewing
			m.addingAttachment = false
			return m, nil

		case key.Matches(msg, keys.Send):
			quoted := fmt.Sprintf(
				"\n\n--- Original Message ---\nFrom: %s\nDate: %s\n\n%s",
				m.replyToMsg.from,
				m.replyToMsg.date,
				indentText(m.currentMsg.body),
			)
			fullBody := m.replyBody.Value() + quoted
			return m, sendEmail(
				m.srv,
				m.replyToMsg.from,
				"",
				"",
				"Re: "+m.replyToMsg.subject,
				fullBody,
				m.replyAttachments,
			)

		case key.Matches(msg, keys.AddAttachment):
			m.addingAttachment = true
			m.attachmentInput.Focus()
			return m, nil

		case key.Matches(msg, keys.RemoveAttachment):
			if len(m.replyAttachments) > 0 {
				m.replyAttachments = m.replyAttachments[:len(m.replyAttachments)-1]
			}
			return m, nil

		case msg.Type == tea.KeyEnter && m.addingAttachment:
			path := m.attachmentInput.Value()
			if _, err := os.Stat(path); err == nil {
				m.replyAttachments = append(m.replyAttachments, path)
				m.addingAttachment = false
				m.attachmentInput.Reset()
			}
			return m, nil
		}
	}

	if m.addingAttachment {
		m.attachmentInput, cmd = m.attachmentInput.Update(msg)
	} else {
		m.replyBody, cmd = m.replyBody.Update(msg)
	}
	return m, cmd
}


func indentText(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

func updateSearching(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch {
        case key.Matches(msg, keys.Back):
            m.state = inbox
            return m, nil

        case msg.Type == tea.KeyEnter:
            m.state = loading
            m.searchQuery = m.searchInput.Value()
            return m, tea.Batch(
                m.loading.Tick,
                performSearch(m.srv, m.searchQuery),
            )
        }
    }

    m.searchInput, cmd = m.searchInput.Update(msg)
    return m, cmd
}

func updateLabelManagement(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			m.state = inbox
			return m, nil

		case key.Matches(msg, keys.Select):
			if selected, ok := m.labelsList.SelectedItem().(labelItem); ok {
				m.state = loading
				return m, tea.Batch(
					m.loading.Tick,
					loadEmailsByLabel(m.srv, selected.label.Id),
				)
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.labelsList, cmd = m.labelsList.Update(msg)
	return m, cmd
}

func loadEmail(srv *gmail.Service, msgID string) tea.Cmd {
	return func() tea.Msg {
		content, err := fetchFullEmailBody(srv, msgID)
		if err != nil {
			return emailLoadErrorMsg{err: err}
		}
		return emailLoadedMsg{content: content}
	}
}

func fetchFullEmailBody(srv *gmail.Service, msgID string) (string, error) {
	msg, err := srv.Users.Messages.Get("me", msgID).Format("full").Do()
	if err != nil {
		return "", fmt.Errorf("failed to fetch message: %w", err)
	}

	var from, subject, date string
	for _, h := range msg.Payload.Headers {
		switch h.Name {
		case "From":
			from = h.Value
		case "Subject":
			subject = h.Value
		case "Date":
			date = formatDate(h.Value)
		}
	}

	body := extractPlainText(msg.Payload)
	if body == "" {
		body = "(no text content found)"
	}

	return fmt.Sprintf("From: %s\nSubject: %s\nDate: %s\n\n%s", 
		from, subject, date, body), nil
}

func sendEmail(srv *gmail.Service, to, cc, bcc, subject, body string, attachments []string) tea.Cmd {
	return func() tea.Msg {
		var msg bytes.Buffer
		msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
		if cc != "" {
			msg.WriteString(fmt.Sprintf("Cc: %s\r\n", cc))
		}
		if bcc != "" {
			msg.WriteString(fmt.Sprintf("Bcc: %s\r\n", bcc))
		}
		msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))

		if len(attachments) == 0 {
			// Simple message without attachments
			msg.WriteString("\r\n" + body)
			raw := base64.URLEncoding.EncodeToString(msg.Bytes())
			_, err := srv.Users.Messages.Send("me", &gmail.Message{Raw: raw}).Do()
			if err != nil {
				return emailLoadErrorMsg{err: err}
			}
			return emailSentMsg{}
		}

		// Multipart message with attachments
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		boundary := writer.Boundary()

		msg.WriteString(fmt.Sprintf("MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%s\r\n\r\n", boundary))
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		msg.WriteString(body + "\r\n")

		// Write the message buffer first
		buf.Write(msg.Bytes())

		// Add attachments
		for _, file := range attachments {
			content, err := ioutil.ReadFile(file)
			if err != nil {
				return emailLoadErrorMsg{err: fmt.Errorf("failed to read attachment: %w", err)}
			}

			partHeader := textproto.MIMEHeader{}
			partHeader.Set("Content-Type", mime.TypeByExtension(filepath.Ext(file)))
			partHeader.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(file)))
			partHeader.Set("Content-Transfer-Encoding", "base64")

			partWriter, err := writer.CreatePart(partHeader)
			if err != nil {
				return emailLoadErrorMsg{err: fmt.Errorf("failed to create attachment part: %w", err)}
			}

			encoder := base64.NewEncoder(base64.StdEncoding, partWriter)
			if _, err := encoder.Write(content); err != nil {
				return emailLoadErrorMsg{err: fmt.Errorf("failed to write attachment: %w", err)}
			}
			encoder.Close()
		}

		writer.Close()
		raw := base64.URLEncoding.EncodeToString(buf.Bytes())
		_, err := srv.Users.Messages.Send("me", &gmail.Message{Raw: raw}).Do()
		if err != nil {
			return emailLoadErrorMsg{err: err}
		}
		return emailSentMsg{}
	}
}


func deleteEmail(srv *gmail.Service, msgId string) tea.Cmd {
    return func() tea.Msg {
        _, err := srv.Users.Messages.Trash("me", msgId).Do()
        if err != nil {
            return emailLoadErrorMsg{err: err}
        }
        return notificationMsg{message: "Email moved to trash"}
    }
}

func toggleReadStatus(srv *gmail.Service, msgId string, isUnread bool) tea.Cmd {
	return func() tea.Msg {
		mod := gmail.ModifyMessageRequest{}
		if isUnread {
			mod.RemoveLabelIds = []string{"UNREAD"}
		} else {
			mod.AddLabelIds = []string{"UNREAD"}
		}

		_, err := srv.Users.Messages.Modify("me", msgId, &mod).Do()
		if err != nil {
			return emailLoadErrorMsg{err: err}
		}
		action := "marked as read"
		if isUnread {
			action = "marked as unread"
		}
		return notificationMsg{message: "Email " + action}
	}
}

func performSearch(srv *gmail.Service, query string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := srv.Users.Messages.List("me").Q(query).MaxResults(30).Do()
		if err != nil {
			return emailLoadErrorMsg{err: err}
		}
		return searchResultMsg{messages: msgs.Messages}
	}
}

func loadLabels(srv *gmail.Service) tea.Cmd {
	return func() tea.Msg {
		labels, err := srv.Users.Labels.List("me").Do()
		if err != nil {
			return emailLoadErrorMsg{err: err}
		}
		return labelsLoadedMsg{labels: labels.Labels}
	}
}

func formatDate(dateStr string) string {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2 Jan 2006 15:04:05 -0700",
	}
	
	for _, format := range formats {
		t, err := time.Parse(format, dateStr)
		if err == nil {
			return t.Format("Jan 02, 2006 15:04")
		}
	}
	return dateStr
}

func extractPlainText(payload *gmail.MessagePart) string {
    if payload.MimeType == "text/plain" && payload.Body != nil && payload.Body.Data != "" {
        return decodeBody(payload.Body.Data)
    }

    if strings.HasPrefix(payload.MimeType, "multipart/") && len(payload.Parts) > 0 {
        for _, p := range payload.Parts {
            if p.MimeType == "text/plain" {
                if text := extractPlainText(p); text != "" {
                    return text
                }
            }
        }

        for _, p := range payload.Parts {
            if p.MimeType == "text/html" {
                if text := extractPlainText(p); text != "" {
                    return text
                }
            }
        }
    }

    if payload.MimeType == "text/html" && payload.Body != nil && payload.Body.Data != "" {
        htmlContent := decodeBody(payload.Body.Data)
        return stripHTML(htmlContent)
    }

    return ""
}

func decodeBody(body string) string {
    if len(body)%4 != 0 {
        body += strings.Repeat("=", (4-len(body)%4)%4)
    }

    decoded, err := base64.URLEncoding.DecodeString(body)
    if err != nil {
        decoded, err = base64.StdEncoding.DecodeString(body)
        if err != nil {
            return "Failed to decode body."
        }
    }
    return string(decoded)
}

func stripHTML(input string) string {
    re := regexp.MustCompile(`<[^>]*>`)
    input = re.ReplaceAllString(input, "")

    entities := map[string]string{
        "&nbsp;": " ", "&lt;": "<", "&gt;": ">", 
        "&amp;": "&", "&quot;": "\"", "&apos;": "'",
    }

    for k, v := range entities {
        input = strings.ReplaceAll(input, k, v)
    }

    input = regexp.MustCompile(`\s+`).ReplaceAllString(input, " ")
    return strings.TrimSpace(input)
}

type notificationMsg struct {
	message string
}

func showNotification(msg string) tea.Cmd {
	return func() tea.Msg {
		return notificationMsg{message: msg}
	}
}

type (
    emailLoadedMsg struct{ content string }
    emailSentMsg   struct{}
    labelsLoadedMsg struct{ labels []*gmail.Label }
    searchResultMsg struct{ messages []*gmail.Message }
)