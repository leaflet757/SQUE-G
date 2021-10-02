package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// ---------------------------------------------------------
// ---------------------------------------------------------
type Logger struct {
    UnPlayableMessages []string
	ArtistMessages     []string
	PlaylistMessages   []string
}

// ---------------------------------------------------------
// ---------------------------------------------------------
func writeMessage(f *os.File, title string, messages *[]string) {
    f.WriteString("--------------------------------\n")
    f.WriteString(fmt.Sprintf("%s\n", title))
    f.WriteString("--------------------------------\n")
    for _, item := range(*messages) {
        f.WriteString(fmt.Sprintf("%s", item))
    }
}

// ---------------------------------------------------------
// ---------------------------------------------------------
func WriteLogs(logger *Logger, config *ConfigData) {
	files, _ := ioutil.ReadDir(config.User.LogsPath)
    filename := fmt.Sprintf("info%d.log", len(files))

    f, _ := os.Create(filepath.Join(config.User.LogsPath, filename))
	
	defer f.Close()

    if len(logger.ArtistMessages) > 0 {
        writeMessage(f, fmt.Sprintf("Artist Date: %s, Total=%d", config.Session.LastRunArtists, len(logger.ArtistMessages)), &logger.ArtistMessages)
	}
    if len(logger.PlaylistMessages) > 0 {
        writeMessage(f, fmt.Sprintf("Playlist Date: %s, Total=%d", config.Session.LastRunPlaylists, len(logger.PlaylistMessages)), &logger.PlaylistMessages)
	}
    if len(logger.UnPlayableMessages) > 0 {
        writeMessage(f, fmt.Sprintf("UnPlayable, Total=%d", len(logger.UnPlayableMessages)), &logger.UnPlayableMessages)
	}
}