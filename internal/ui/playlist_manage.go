package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Benehiko/tidalt/v4/internal/tidal"
)

// playlistNamePurpose says what the name prompt will do on Enter.
type playlistNamePurpose int

const (
	nameForEmptyPlaylist playlistNamePurpose = iota // create an empty playlist
	nameForQueueSave                                // save the queue as a new playlist
)

type playlistCreatedMsg struct {
	uuid string
	name string
}

type playlistDeletedMsg struct {
	uuid string
	name string
}

// openPlaylistNamePrompt raises the name prompt, prefilled with initial.
func (m *Model) openPlaylistNamePrompt(purpose playlistNamePurpose, initial string) {
	ti := textinput.New()
	ti.Placeholder = "Playlist name"
	ti.Prompt = ""
	ti.CharLimit = 200
	ti.SetValue(initial)
	ti.CursorEnd()
	ti.Focus()
	m.nameInput = ti
	m.namePurpose = purpose
	m.nameError = ""
	m.overlay = OverlayPlaylistName
}

func (m Model) updatePlaylistName(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case keyEsc:
		m.overlay = OverlayNone
		return m, nil
	case keyEnter:
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			m.nameError = "Enter a name."
			return m, nil
		}
		m.overlay = OverlayNone
		if m.namePurpose == nameForQueueSave {
			return m, m.saveQueueCmd(name)
		}
		return m, m.createPlaylistCmd(name)
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(k)
	return m, cmd
}

func (m *Model) renderPlaylistName(t Theme) string {
	w := min(max(m.width/2, 40), 60)
	title := "NEW PLAYLIST"
	hint := " ↵ create · Esc cancel"
	if m.namePurpose == nameForQueueSave {
		title = "SAVE QUEUE AS PLAYLIST"
		hint = fmt.Sprintf(" ↵ save %d tracks · Esc cancel", len(m.tracks))
	}
	rows := []string{
		" " + t.KeyBarKey.Render("≡ ") + m.nameInput.View(),
		"",
		t.RowDim.Render(hint),
	}
	if m.nameError != "" {
		rows = append(rows, "", t.Err.Render(" "+m.nameError))
	}
	return renderPanel(t, title, true, w, len(rows)+2, strings.Join(rows, "\n"))
}

func (m *Model) createPlaylistCmd(name string) tea.Cmd {
	client, ctx := m.client, m.ctx
	return func() tea.Msg {
		uuid, err := client.CreatePlaylist(ctx, name, "")
		if err != nil {
			return errMsg(err)
		}
		return playlistCreatedMsg{uuid: uuid, name: name}
	}
}

// openDeletePlaylist asks for confirmation before deleting pl.
func (m *Model) openDeletePlaylist(pl tidal.Playlist) {
	m.deleteTarget = &pl
	m.overlay = OverlayConfirmDelete
}

// updateConfirmDelete only deletes on an explicit "y"; anything else that
// isn't ignored cancels, so a stray Enter can't destroy a playlist.
func (m Model) updateConfirmDelete(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "y", "Y":
		pl := m.deleteTarget
		m.overlay = OverlayNone
		m.deleteTarget = nil
		if pl == nil {
			return m, nil
		}
		client, ctx := m.client, m.ctx
		uuid, name := pl.UUID, pl.Title
		return m, func() tea.Msg {
			if err := client.DeletePlaylist(ctx, uuid); err != nil {
				return errMsg(err)
			}
			return playlistDeletedMsg{uuid: uuid, name: name}
		}
	case "n", "N", keyEsc, keyEnter:
		m.overlay = OverlayNone
		m.deleteTarget = nil
	}
	return m, nil
}

func (m *Model) renderConfirmDelete(t Theme) string {
	if m.deleteTarget == nil {
		return ""
	}
	w := min(max(m.width/2, 40), 60)
	pl := m.deleteTarget
	rows := []string{
		" " + truncateStr(fmt.Sprintf("Delete “%s” (%d tracks)?", pl.Title, pl.NumberOfTracks), w-3),
		t.RowDim.Render(" This removes it from your TIDAL account."),
		"",
		" " + t.KeyBarKey.Render("y") + t.RowDim.Render(" delete · ") + t.KeyBarKey.Render("n") + t.RowDim.Render(" cancel"),
	}
	return renderPanel(t, "DELETE PLAYLIST", true, w, len(rows)+2, strings.Join(rows, "\n"))
}

// reloadPlaylistsCmd drops the cached playlist list and fetches it again.
func (m *Model) reloadPlaylistsCmd() tea.Cmd {
	m.playlists = nil
	return m.loadSection(SecPlaylists)
}

// handlePlaylistCreated refreshes the list after a create.
func (m *Model) handlePlaylistCreated(msg playlistCreatedMsg) tea.Cmd {
	m.toast = fmt.Sprintf("✓ Created %q", msg.name)
	m.cursor = 0 // the list is newest-first, so the new playlist lands on top
	return tea.Batch(m.reloadPlaylistsCmd(), toastClearCmd())
}

// handlePlaylistDeleted drops the playlist locally and detaches anything that
// still points at it.
func (m *Model) handlePlaylistDeleted(msg playlistDeletedMsg) tea.Cmd {
	for i := range m.playlists {
		if m.playlists[i].UUID == msg.uuid {
			m.playlists = append(m.playlists[:i], m.playlists[i+1:]...)
			break
		}
	}
	if m.section == SecPlaylists && m.cursor >= len(m.playlists) {
		m.cursor = max(len(m.playlists)-1, 0)
	}
	if m.openPlaylist != nil && m.openPlaylist.UUID == msg.uuid {
		m.openPlaylist = nil
		m.detailTracks = nil
		m.detailFocus = false
	}
	// The queue keeps its tracks but no longer belongs to a saved playlist.
	if m.queuePlaylistUUID == msg.uuid {
		m.queuePlaylistUUID = ""
		m.queueSource = ""
		m.queueDirty = false
	}
	m.toast = fmt.Sprintf("✓ Deleted %q", msg.name)
	return toastClearCmd()
}

// textEntryActive reports whether keystrokes are currently going into a text
// field, so global single-letter shortcuts (q, t, :) must not fire.
func (m *Model) textEntryActive() bool {
	switch m.overlay {
	case OverlayCommandPalette, OverlayPlaylistName:
		return true
	case OverlayImportSpotify:
		return m.importStage == importStageInput
	}
	return m.searchInput.Focused()
}
