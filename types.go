package main

import "time"

// ---------------------------------------------------------
// Constants
// ---------------------------------------------------------

const SQUE_DATE_FORMAT             = "2006-01-02" // '2006' for YYYY, '01' for MM, '02' for DD, Equivalent to YYYY-MM-DD
const SQUE_ALERT_STALE_PLAYLIST    = 6 // in months

const SQUE_SPOTIFY_LIMIT_TRACKS    = 20
const SQUE_SPOTIFY_LIMIT_ARTISTS   = 50
const SQUE_SPOTIFY_LIMIT_ALBUMS    = 50
const SQUE_SPOTIFY_LIMIT_PLAYLISTS = 100
const SQUE_SPOTIFY_MARKET          = "US"

// ---------------------------------------------------------
// User Meta Data
// ---------------------------------------------------------

type UserData struct {
	ClientID 		    string `json:"client_id"`
	ClientSecret		string `json:"client_secret"`
	RedirectURI 		string `json:"redirect_uri"`
	LogsPath			string `json:"logs_path"`
	PlaylistListenLater string `json:"listen_later"`
	PlaylistCompilation string `json:"compilation"`
	PlaylistSets		string `json:"sets"`
}

type SessionFlags uint8
const (
	SessionFlags_ScanPlaylists SessionFlags = 1 << iota
	SessionFlags_ScanArtists
)

type SessionData struct {
	Flags            SessionFlags
	CurrentDateTime  time.Time
	LastRunArtists   time.Time
	LastRunPlaylists time.Time
}

type PlaylistMetaData struct {
	ID	  string `json:"id"`
	Name  string `json:"name"`
	Limit string `json:"limit"` // TODO: Convert to int
}

type ConfigData struct {
	User 	    UserData
	Session     SessionData
	Playlists []PlaylistMetaData
}

// ---------------------------------------------------------
// Queuer Types
// ---------------------------------------------------------

type Track struct {
	URI 	 string
	Name  	 string
	Artist   int
	Album    int
	Playlist int
	Score	 int
	DateTime time.Time
}

type AlbumType int
const (
	AlbumType_Album       AlbumType = iota
	AlbumType_Compilation
	AlbumType_Single
	AlbumType_AppearsOn
)

type Album struct {
	ID 			  string
	Name 	      string
	Type 	      AlbumType
	Artist        int
	ReleaseDate   time.Time
	Tracks      []int
}

type Playlist struct {
	ID 			     string
	Name             string
	PlaylistMetaID   int
	Tracks         []int
}

type Artist struct {
	ID       string
	Name     string
	Albums []int
}

type Cache struct {
	TrackDatas       []Track
	TrackDatasMap    map[string]int

	AlbumDatas       []Album
	AlbumDatasMap    map[string]int
	
	PlaylistDatas    []Playlist
	PlaylistDatasMap map[string]int
	
	ArtistDatas      []Artist
	ArtistDatasMap   map[string]int
}

type TrackAdder struct {
	ListenLater  []int
	Sets         []int
	Compilations []int
}