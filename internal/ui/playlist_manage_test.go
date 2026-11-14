package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Benehiko/tidalt/v4/internal/tidal"
)

func key(s string) tea.KeyMsg {
	switch s {
	case keyEnter:
		return tea.KeyMsg{Type: tea.KeyEnter}
	case keyEsc:
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func playlistsModel() Model {
	m := newSmokeModel()
	m.section = SecPlaylists
	m.playlists = []tidal.Playlist{
		{UUID: "a", Title: "Alpha", NumberOfTracks: 3},
		{UUID: "b", Title: "Bravo", NumberOfTracks: 5},
	}
	return m
}

// Typing a name must not trigger the global q (quit) / t (theme) shortcuts.
func TestNamePromptTakesTypedLetters(t *testing.T) {
	m := playlistsModel()
	mm, _ := m.handleKey(key("n"))
	m = asModel(t, mm)
	if m.overlay != OverlayPlaylistName {
		t.Fatalf("n should open the name prompt, overlay=%v", m.overlay)
	}
	theme := m.themeName
	// If a global shortcut swallowed q or t, it would be missing from the
	// value (and the app would have quit / changed theme).
	for _, r := range "quiet tunes" {
		mm, _ := m.handleKey(key(string(r)))
		m = asModel(t, mm)
	}
	if got := m.nameInput.Value(); got != "quiet tunes" {
		t.Errorf("name = %q, want %q", got, "quiet tunes")
	}
	if m.themeName != theme {
		t.Errorf("typing 't' changed the theme to %q", m.themeName)
	}
}

func TestNamePromptRejectsEmpty(t *testing.T) {
	m := playlistsModel()
	m.openPlaylistNamePrompt(nameForEmptyPlaylist, "  ")
	mm, cmd := m.handleKey(key(keyEnter))
	m = asModel(t, mm)
	if cmd != nil || m.overlay != OverlayPlaylistName || m.nameError == "" {
		t.Errorf("blank name should stay open with an error (overlay=%v err=%q)", m.overlay, m.nameError)
	}
}

func TestDeleteNeedsExplicitYes(t *testing.T) {
	m := playlistsModel()
	m.cursor = 1
	mm, _ := m.handleKey(key("x"))
	m = asModel(t, mm)
	if m.overlay != OverlayConfirmDelete || m.deleteTarget == nil || m.deleteTarget.UUID != "b" {
		t.Fatalf("x should ask to delete Bravo, got overlay=%v target=%+v", m.overlay, m.deleteTarget)
	}
	// Enter cancels rather than confirming.
	mm, cmd := m.handleKey(key(keyEnter))
	m = asModel(t, mm)
	if cmd != nil || m.overlay != OverlayNone {
		t.Fatalf("enter should cancel without deleting")
	}
	m.openDeletePlaylist(m.playlists[1])
	if _, cmd := m.handleKey(key("y")); cmd == nil {
		t.Fatal("y should issue the delete command")
	}
}

func TestPlaylistDeletedDetachesState(t *testing.T) {
	m := playlistsModel()
	m.cursor = 1
	pl := m.playlists[1]
	m.openPlaylist = &pl
	m.detailTracks = m.tracks
	m.queuePlaylistUUID = "b"
	m.queueSource = "playlist:Bravo"

	m.handlePlaylistDeleted(playlistDeletedMsg{uuid: "b", name: "Bravo"})

	if len(m.playlists) != 1 || m.playlists[0].UUID != "a" {
		t.Errorf("playlists = %+v", m.playlists)
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	if m.openPlaylist != nil || m.detailTracks != nil {
		t.Error("deleted playlist still open in the detail pane")
	}
	if m.queueSource != "" || m.queuePlaylistUUID != "" {
		t.Error("queue still claims to belong to the deleted playlist")
	}
	if len(m.tracks) == 0 {
		t.Error("queue tracks should be kept")
	}
}

func TestOverlaysRender(t *testing.T) {
	m := playlistsModel()
	m.width, m.height = 100, 30
	m.openPlaylistNamePrompt(nameForQueueSave, "Mix")
	if m.renderPlaylistName(m.theme) == "" {
		t.Error("name prompt rendered empty")
	}
	m.openDeletePlaylist(m.playlists[0])
	if m.renderConfirmDelete(m.theme) == "" {
		t.Error("delete confirmation rendered empty")
	}
}
