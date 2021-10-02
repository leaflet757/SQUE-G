package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"io/ioutil"
	"os"
	"time"
	"sort"
	"strings"

	"github.com/zmb3/spotify/v2"
)

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
// User and Session Data
// ---------------------------------------------------------

type UserData struct {
	UserID 			    string
	UserDataPath        string
	ClientID 		    string `json:"client_id"`
	ClientSecret		string `json:"client_secret"`
	RedirectURI 		string `json:"redirect_uri"`
	LogsPath			string `json:"logs_path"`
	LastRunPath			string `json:"last_run_path"`
	PlaylistListenLater string `json:"listen_later"`
	PlaylistCompilation string `json:"compilation"`
	PlaylistSets		string `json:"sets"`
}

type SessionFlags uint8
const (
	SessionFlags_ScanPlaylists SessionFlags = 1 << iota
	SessionFlags_ScanArtists
	SessionFlags_PrintFollowedPlaylists
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
	Limit int    `json:"limit"`
}

func (p *PlaylistMetaData) Unbounded() bool {
	return p.Limit == -1
}

type ConfigData struct {
	User 	     UserData
	Session      SessionData
	Playlists  []PlaylistMetaData
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
	UnPlayable   []int
}

// ---------------------------------------------------------
// ---------------------------------------------------------
func InitCache(c *Cache) {
	c.TrackDatasMap    = make(map[string]int)
	c.AlbumDatasMap    = make(map[string]int)
	c.PlaylistDatasMap = make(map[string]int)
	c.ArtistDatasMap   = make(map[string]int)
}

// ---------------------------------------------------------
// ---------------------------------------------------------
func InitConfigData(c *ConfigData, userDataPath string) {
	// Load json
	configBytes, configErr := ioutil.ReadFile(userDataPath)
	if configErr != nil {
		log.Fatalf("Could not load user data path: %s\n", configErr)
	}
	
	// unmarshall it, copy to object
	configErr = json.Unmarshal(configBytes, c)
	if configErr != nil {
		log.Fatalf("%s\n", configErr)
	}
	
	c.User.UserDataPath = userDataPath

	lastRunArtists, lastRunPlaylists := parseLastRunFile(c.User.LastRunPath)

	// Artists last run
	dateTime, timeErr := time.Parse(SQUE_DATE_FORMAT, lastRunArtists)
	if timeErr != nil {
		log.Fatalf("Could not parse Artist date: %s\n", timeErr)
	}
	c.Session.LastRunArtists = dateTime

	// Playlists last run
	dateTime, timeErr = time.Parse(SQUE_DATE_FORMAT, lastRunPlaylists)
	if timeErr != nil {
		log.Fatalf("Could not parse Playlist date: %s\n", timeErr)
	}
	c.Session.LastRunPlaylists = dateTime

	fmt.Printf("Last run artists: %s\n", c.Session.LastRunArtists)
	fmt.Printf("Last run playlists: %s\n", c.Session.LastRunPlaylists)

	// Current
	c.Session.CurrentDateTime = time.Now()
}

// ---------------------------------------------------------
// ---------------------------------------------------------
func CloseAndSave(c *ConfigData) {
    f, err := os.Create(c.User.LastRunPath)
    if err != nil {
        log.Fatal(err)
    }

	defer f.Close()

	// artists must go first
	if (config.Session.Flags & SessionFlags_ScanArtists) != 0 {
		f.WriteString(c.Session.CurrentDateTime.Format(SQUE_DATE_FORMAT))
	} else {
		f.WriteString(c.Session.LastRunArtists.Format(SQUE_DATE_FORMAT))
	}

	// delimeter
	f.WriteString(",")

	// playlists must go second
	if (config.Session.Flags & SessionFlags_ScanPlaylists) != 0 {
		f.WriteString(c.Session.CurrentDateTime.Format(SQUE_DATE_FORMAT))
	} else {
		f.WriteString(c.Session.LastRunPlaylists.Format(SQUE_DATE_FORMAT))
	}
}

// ---------------------------------------------------------
// ---------------------------------------------------------
func parseLastRunFile(path string) (string, string) {
	data, err := ioutil.ReadFile(path)
	
	if err != nil {
		log.Fatal(err)
	}
	
	result := strings.Split(string(data), ",")
	lastRunArtistsDateStr   := result[0]
	lastRunPlaylistsDateStr := result[1]
	
	return lastRunArtistsDateStr, lastRunPlaylistsDateStr
}

// ---------------------------------------------------------
// ---------------------------------------------------------
func CheckOption(config *ConfigData, argv []string, index int) {
	if argv[index] == "-a" { // Scan Artists
		config.Session.Flags |= SessionFlags_ScanArtists
	} else if argv[index] == "-p" { // Scan Playlists
		config.Session.Flags |= SessionFlags_ScanPlaylists
	} else if argv[index] == "-fp" { // Print Followed Playlists
		config.Session.Flags |= SessionFlags_PrintFollowedPlaylists
	} else if argv[index] == "-d" {
		storeArtist := false
		storePlaylist := false
		for i := 1; i < len(argv); i++ {
			if argv[i] == "-a" {
				storeArtist = true
			}
			if argv[i] == "-p" {
				storePlaylist = true
			}
		}

		dateTime, timeErr := time.Parse(SQUE_DATE_FORMAT, argv[index + 1])
		if timeErr != nil {
			log.Fatalf("Could not parse debug date: %s\n", timeErr)
		}

		if storeArtist {
			fmt.Printf("Overwriting last run artist date %s. Writing new artist date %s.", config.Session.LastRunArtists, dateTime)
			config.Session.LastRunArtists = dateTime
		}
		if storePlaylist {
			fmt.Printf("Overwriting last run playlist date %s. Writing new playlist date %s.", config.Session.LastRunPlaylists, dateTime)
			config.Session.LastRunPlaylists = dateTime
		}
	}
}

// ---------------------------------------------------------
// ---------------------------------------------------------
func ScanArtistTracks(client *spotify.Client, cache *Cache, config *ConfigData, adder *TrackAdder) {
	fmt.Println("Scanning Artists....")

	albumType := []spotify.AlbumType{spotify.AlbumTypeAlbum, spotify.AlbumTypeSingle, spotify.AlbumTypeCompilation/*, spotify.AlbumTypeAppearsOn*/} // we dont care about 'AppearsOn'
	
	// Followed Artists
	artists, artistErr := client.CurrentUsersFollowedArtists(context.Background(), spotify.Limit(SQUE_SPOTIFY_LIMIT_ARTISTS))
	
	continueScanning := (artistErr == nil)
	
	for len(artists.Artists) > 0 && continueScanning {
		fmt.Println("Artist Limit:", artists.Limit)
		fmt.Println("Number of artists:", len(artists.Artists))
		
		for _, artist := range artists.Artists {
			artistDataIndex, ok := cache.ArtistDatasMap[artist.ID.String()]

			// Artist data does not exist, create it
			if !ok {
				artistDataIndex = len(cache.ArtistDatas)
				cache.ArtistDatasMap[artist.ID.String()] = artistDataIndex
				cache.ArtistDatas = append(cache.ArtistDatas, 
					Artist {
						ID:   artist.ID.String(),
						Name: artist.Name,
				})
			}
			
			var simpleTracksToAdd []int
			artistData := cache.ArtistDatas[artistDataIndex]
			fmt.Printf(">>>%s\n", artistData.Name)

			// Get the artist's albums
			artistAlbums, albumsErr := client.GetArtistAlbums(context.Background(), spotify.ID(artistData.ID), albumType, spotify.Limit(SQUE_SPOTIFY_LIMIT_ALBUMS))

			for len(artistAlbums.Albums) > 0 && albumsErr == nil {
				for _, album := range artistAlbums.Albums {
					/*
					*  Some 'Compilation' spotify albums will be marked as compilation
                    *  even though we really want them in listen later playlist. But 
                    *  some compilations are actual compilations of many artists. So if
                    *  this album has a bunch of artists, its most likely a compilation.
                    *  This will probably skip cool older songs tho :'(
					*/
					if album.AlbumGroup == "appears_on" {
						continue
					}

					// Get the album release date, skip album if its older than our last run
					// For some reason, Spotify will sometimes return songs that haven't been officially released yet.
					// So skip songs also that have a release date after the current date time
					albumReleaseDateTime := album.ReleaseDateTime()
					if albumReleaseDateTime.Before(config.Session.LastRunArtists) || albumReleaseDateTime.After(config.Session.CurrentDateTime) {
						continue
					}

					albumDataIndex, ok := cache.AlbumDatasMap[album.ID.String()]

					// Album data does not exist, create it
					if !ok {
						albumType := AlbumType_Album // assume "album"
						if album.AlbumType == "single" {
							albumType = AlbumType_Single
						} else if album.AlbumType == "compilation" {
							albumType = AlbumType_Compilation
						}
						
						albumDataIndex = len(cache.AlbumDatas)
						cache.AlbumDatasMap[album.ID.String()] = albumDataIndex
						cache.AlbumDatas = append(cache.AlbumDatas, 
							Album {
								ID:          album.ID.String(),
								Name:        album.Name,
								Type:        albumType,
								Artist:      artistDataIndex,
								ReleaseDate: albumReleaseDateTime,
						})
						artistData.Albums = append(artistData.Albums, albumDataIndex)
					}
					albumData := cache.AlbumDatas[albumDataIndex]

					// Get the album's tracks
					albumTracks, tracksErr := client.GetAlbumTracks(context.Background(), album.ID, spotify.Limit(SQUE_SPOTIFY_LIMIT_TRACKS), spotify.Market(SQUE_SPOTIFY_MARKET))
					
					for len(albumTracks.Tracks) > 0 && tracksErr == nil {						
						for _, track := range albumTracks.Tracks {
							// Skip tracks that are 'intro' tracks that dont really have much music content
                    		// 80s = 80000ms
							if track.Duration <= 80000 {
								continue
							}

							// if we already have the track then skip
							trackDataIndex, ok := cache.TrackDatasMap[track.ID.String()]
							if ok { 
								continue
							}

							// Create the track data
							trackDataIndex = len(cache.TrackDatas)
							cache.TrackDatasMap[track.ID.String()] = trackDataIndex
							cache.TrackDatas = append(cache.TrackDatas, 
								Track {
									URI: string(track.URI),
									Name: track.Name,
									Artist: artistDataIndex,
									Album: albumDataIndex,
									Playlist: -1, // not from a playlist
									Score: 0, // dont care about score of artists we follow, we want em all
									DateTime: albumReleaseDateTime,
							})
							albumData.Tracks = append(albumData.Tracks, trackDataIndex)
							
							// Add the simple track so we can query the full track later.
							simpleTracksToAdd = append(simpleTracksToAdd, trackDataIndex)
						}

						tracksErr = client.NextPage(context.Background(), albumTracks)
					}
				}
				albumsErr = client.NextPage(context.Background(), artistAlbums)
			}
			
			// Artists will release music under different licenses that may or may not
			// allow returned songs from the spotify api to be playable by the current
			// user. So we need to pull data of the full track to see if its playable
			toAddIndex := 0
			totalTracks := len(simpleTracksToAdd)

			for toAddIndex < totalTracks {
				chunkLen := SQUE_SPOTIFY_LIMIT_ARTISTS
				
				if (toAddIndex + chunkLen) > totalTracks {
					chunkLen = totalTracks - toAddIndex
				}

				trackChunk := make([]spotify.ID, chunkLen)
				subtracks := simpleTracksToAdd[toAddIndex:toAddIndex+chunkLen]
		
				// Create the spotify ids for all the possible tracks
				for trackDataIndex, trackDataID := range(subtracks) {
					trackData := cache.TrackDatas[trackDataID]
		
					if strings.Contains(trackData.URI, "spotify:track:") {
						trackChunk[trackDataIndex] = spotify.ID(trackData.URI[len("spotify:track:"):len(trackData.URI)])
					} else {
						trackChunk[trackDataIndex] = spotify.ID(trackData.URI)
					}
				}

				// Get the full track
				fullTracks, fullTrackErr := client.GetTracks(context.Background(), trackChunk, spotify.Market(SQUE_SPOTIFY_MARKET))
				
				if fullTrackErr != nil {
					log.Fatal(fullTrackErr)
				}

				for _, track := range(fullTracks) {
					// Key to data map must exist if we got here
					trackDataIndex, _ := cache.TrackDatasMap[track.ID.String()]

					trackData := cache.TrackDatas[trackDataIndex]
					albumData := cache.AlbumDatas[trackData.Album]
					artistData := cache.ArtistDatas[trackData.Artist]

					// Not that it matters, we want the song anyway... but grab the score
					trackData.Score = track.Popularity

					if *track.IsPlayable {
						// The track is playable and can be added
						if track.Duration >= 1860000 {
							adder.Sets = append(adder.Sets, trackDataIndex)
						} else {
							adder.ListenLater = append(adder.ListenLater, trackDataIndex)
						}
						fmt.Printf("  *%s\n", trackData.Name)
						logger.ArtistMessages = append(logger.ArtistMessages, fmt.Sprintf("%s --- %s --- %s --- %s --- %d --- %v\n", artistData.Name, albumData.Name, albumData.ReleaseDate.String(), trackData.Name, trackData.Score, track.AvailableMarkets))
					} else {
						// The track is unplayable for some reason
						adder.UnPlayable = append(adder.UnPlayable, trackDataIndex)
						logger.UnPlayableMessages = append(logger.UnPlayableMessages, fmt.Sprintf("%s --- %s --- %s --- %s --- %d --- %v\n", artistData.Name, albumData.Name, albumData.ReleaseDate.String(), trackData.Name, trackData.Score, track.AvailableMarkets))
					}
				}

				// chunk scan complete
				toAddIndex += chunkLen
			}
		}

		// artist page complete
		fmt.Println("Cursor After:", artists.Cursor.After)
		if len(artists.Cursor.After) > 0 {
			artists, artistErr = client.CurrentUsersFollowedArtists(context.Background(), spotify.Limit(SQUE_SPOTIFY_LIMIT_ARTISTS), spotify.After(artists.Cursor.After) )
			continueScanning = artistErr == nil
		} else {
			continueScanning = false
		}
	}
}

func ScanPlaylistTracks(client *spotify.Client, cache *Cache, config *ConfigData, adder *TrackAdder) {
	fmt.Println("Scanning Playlists....")
		
	for playlistMetaIndex, playlistMeta := range(config.Playlists) {
		var sortedPlaylistTracks []int

		// Check if this playlists PlaylistData exists
		playlistDataIndex, ok := cache.PlaylistDatasMap[playlistMeta.ID]
		if !ok {
			playlistDataIndex = len(cache.PlaylistDatas)
			cache.PlaylistDatasMap[playlistMeta.ID] = playlistDataIndex
			cache.PlaylistDatas = append(cache.PlaylistDatas, 
				Playlist {
					ID:             playlistMeta.ID,
					Name:           playlistMeta.Name,
					PlaylistMetaID: playlistMetaIndex,
			})
		}
		playlistData := cache.PlaylistDatas[playlistDataIndex]
		
		fmt.Printf(">>>%s\n", playlistData.Name)

		playlistTracks, playlistErr := client.GetPlaylistTracks(context.Background(), spotify.ID(playlistMeta.ID), spotify.Limit(SQUE_SPOTIFY_LIMIT_TRACKS), spotify.Market(SQUE_SPOTIFY_MARKET))
		scanPlaylist := (playlistErr == nil || playlistErr == spotify.ErrNoMorePages)

		for len(playlistTracks.Tracks) > 0 && scanPlaylist {
			for _, playlistTrack := range(playlistTracks.Tracks) {

				// Check the release date of the track
				trackReleaseDateTime, dateTimeErr := time.Parse(spotify.TimestampLayout, playlistTrack.AddedAt)
				if dateTimeErr != nil {
					fmt.Printf("Cannot determine date for playlist track %s\n", playlistTrack.Track.Name)
					continue
				}
				
				// Skip track if the song was added previously when we ran this script
				if trackReleaseDateTime.Before(config.Session.LastRunPlaylists) {
					continue
				}

				// Check if the TrackData exists for this track
				trackDataIndex, ok := cache.TrackDatasMap[playlistTrack.Track.ID.String()]

				if !ok {
					trackDataIndex = len(cache.TrackDatas)
					cache.TrackDatasMap[playlistTrack.Track.ID.String()] = trackDataIndex
					cache.TrackDatas = append(cache.TrackDatas, 
						Track {
							URI: string(playlistTrack.Track.URI),
							Name: playlistTrack.Track.Name,
							Artist: -1,
							Album: -1,
							Playlist: playlistDataIndex,
							Score: playlistTrack.Track.Popularity,
							DateTime: trackReleaseDateTime,
					})
				}

				playlistData.Tracks = append(playlistData.Tracks, trackDataIndex)

				sortedPlaylistTracks = append(sortedPlaylistTracks, trackDataIndex)
			}

			playlistErr = client.NextPage(context.Background(), playlistTracks)
			scanPlaylist = playlistErr == nil
		}

		if playlistErr != nil && playlistErr != spotify.ErrNoMorePages {
			log.Fatal(playlistErr)
		}

		// Sort the possible tracks to add from this playlist by their popularity.
		// Prefer the more popular songs
		sort.SliceStable(sortedPlaylistTracks, func(i, j int) bool {
			return cache.TrackDatas[i].Score > cache.TrackDatas[j].Score
		})

		// Add some number of songs from this playlists as defined by the user's user data
		for i := 0; i < len(sortedPlaylistTracks); i++ {
			if !(playlistMeta.Unbounded() || i < playlistMeta.Limit) {
				fmt.Printf("!!!Hit limit %d for %s\n", playlistMeta.Limit, playlistMeta.Name)
				break
			}

			trackDataIndex := sortedPlaylistTracks[i]
			trackData := cache.TrackDatas[trackDataIndex]
			fmt.Printf("  *%s\n", trackData.Name)

			logger.PlaylistMessages = append(logger.PlaylistMessages, fmt.Sprintf("%s --- %s --- %s --- %d\n", playlistData.Name, trackData.DateTime, trackData.Name, trackData.Score))

			adder.ListenLater = append(adder.ListenLater, trackDataIndex)
		}
	}
}

func AddTracksToPlaylist(client *spotify.Client, cache *Cache, playlistId string, tracks []int) {
	totalTracks := len(tracks)

	if totalTracks == 0 {
		return
	}

	spotPlaylistID := spotify.ID(playlistId)
	
	trackIndex := 0
	
	for trackIndex < totalTracks {
		chunkLength := SQUE_SPOTIFY_LIMIT_PLAYLISTS
		
		if (trackIndex + chunkLength) > totalTracks {
			chunkLength = totalTracks - trackIndex
		}
				
		trackchunk := make([]spotify.ID, chunkLength)
		subtracks := tracks[trackIndex:trackIndex+chunkLength]

		for trackDataIndex, trackDataID := range(subtracks) {
			trackData := cache.TrackDatas[trackDataID]

			if strings.Contains(trackData.URI, "spotify:track:") {
				trackchunk[trackDataIndex] = spotify.ID(trackData.URI[len("spotify:track:"):len(trackData.URI)])
			} else {
				trackchunk[trackDataIndex] = spotify.ID(trackData.URI)
			}
		}

		_, err := client.AddTracksToPlaylist(context.Background(), spotPlaylistID, trackchunk...)
		if err != nil {
			log.Fatal(err)
		}

		trackIndex += chunkLength	
	}
}

func ShowFollowedPlaylists(client *spotify.Client, config *ConfigData) {
	playlistPage, err := client.GetPlaylistsForUser(context.Background(), config.User.UserID)

	if err != nil {
		log.Fatal(err)
	}

	scanPlaylist := true

	for len(playlistPage.Playlists) > 0 && scanPlaylist {

		for i := 0; i < len(playlistPage.Playlists); i++ {
			playlist := playlistPage.Playlists[i]
			fmt.Printf("%s -- %s\n", playlist.ID, playlist.Name)
		}

		playlistErr := client.NextPage(context.Background(), playlistPage)
		scanPlaylist = playlistErr == nil
	}

}