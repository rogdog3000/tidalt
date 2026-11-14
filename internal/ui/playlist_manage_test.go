package ui

import (
	"strings"
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

// Top tracks open as a browsable list (like an album) and Esc returns to the
// artist view, rather than the tracks replacing the queue.
func TestArtistTopTracksOpensList(t *testing.T) {
	m := newSmokeModel()
	m.showArtist = true
	m.artistID = 9
	m.artistName = "Pierce The Veil"
	m.artistCursor = 1
	queue := len(m.tracks)

	mm, cmd := m.handleKey(key(keyEnter))
	m = asModel(t, mm)
	if cmd == nil || !m.artistLoading {
		t.Fatal("Enter on Top tracks should start loading them")
	}

	top := []tidal.Track{{ID: 7, Title: "Caraphernelia"}, {ID: 8, Title: "Bulls In The Bronx"}}
	mm, _ = m.Update(artistAlbumTracksMsg{album: tidal.Album{Title: topTracksTitle}, tracks: top})
	m = asModel(t, mm)
	if m.artistAlbum == nil || len(m.artistAlbumTracks) != 2 {
		t.Fatalf("top tracks not shown as a list: %+v", m.artistAlbum)
	}
	if len(m.tracks) != queue {
		t.Error("queue changed just by opening top tracks")
	}
	m.width, m.height = 100, 30
	if got := m.renderArtistAlbumPane(m.theme, 80, 20); !strings.Contains(got, "TOP TRACKS") {
		t.Error("pane title should say TOP TRACKS")
	}

	mm, _ = m.handleKey(key(keyEsc))
	m = asModel(t, mm)
	if m.artistAlbum != nil || !m.showArtist {
		t.Error("Esc should return to the artist view")
	}
}
