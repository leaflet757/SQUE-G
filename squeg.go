// https://github.com/zmb3/spotify/blob/master/track.go

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"io/ioutil"
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
	Limit string `json:"limit"` // TODO: Convert to int
}

func (p *PlaylistMetaData) Unbounded() bool {
	return p.Limit == -1
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

func InitCache(c *Cache) {
	c.TrackDatasMap    = make(map[string]int)
	c.AlbumDatasMap    = make(map[string]int)
	c.PlaylistDatasMap = make(map[string]int)
	c.ArtistDatasMap   = make(map[string]int)
}

func InitConfigData(c *ConfigData, userDataPath string, lastRunFilePath string) {
	// Load json
	configBytes, configErr := ioutil.ReadFile(userDataPath)
	if configErr != nil {
		log.Fatalf("Could not load user data path: %s\n", configErr)
	}

	// unmarshall it, copy to object
	configErr = json.Unmarshal(configBytes, c)
	if configErr != nil {
		log.Fatalf("could not unmarshal config data: %s\n", configErr)
	}

	lastRunArtists, lastRunPlaylists := parseLastRunFile(lastRunFilePath)

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

func ScanArtistTracks(client *spotify.Client, cache *Cache, config *ConfigData, adder *TrackAdder) {
	fmt.Println("Scanning Artists....")

	dbgCount := 0

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
							
							// Add the track to the listen later playlist
							adder.ListenLater = append(adder.ListenLater, trackDataIndex)
							
							trackData := cache.TrackDatas[trackDataIndex]
							fmt.Printf("  *%s\n", trackData.Name)

							// TODO: Logger
							//fmt.Printf("%s --- %s --- %s --- %s\n", artistData.Name, albumData.Name, albumData.ReleaseDate.String(), trackData.Name)

							if dbgCount > 0 {
								return
							}
							dbgCount++
						}

						tracksErr = client.NextPage(context.Background(), albumTracks)
					}
				}
				albumsErr = client.NextPage(context.Background(), artistAlbums)
			}
		}
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
	
	dbgCount := 0
	
	// TODO: Playlist track sorting
	
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
				// TODO, should skip if we have the song
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
				//trackData := cache.TrackDatas[trackDataIndex]

				sortedPlaylistTracks = append(sortedPlaylistTracks, trackDataIndex)

				if dbgCount > 10 {
					return
				}
				dbgCount++
			}

			playlistErr = client.NextPage(context.Background(), playlistTracks)
			scanPlaylist = playlistErr == nil
		}

		if playlistErr != nil && playlistErr != spotify.ErrNoMorePages {
			log.Fatal(playlistErr)
		}

		sort.SliceStable(sortedPlaylistTracks, func(i, j int) bool {
			return cache.TrackDatas[i].Score > cache.TrackDatas[j].Score
		})

		for i := 0; i < len(sortedPlaylistTracks); i++ {
			if !(playlistMeta.Unbounded() || i < playlistMeta.Limit) {
				fmt.Printf("!!!Hit limit %d for %s", playlistMeta.Limit, playlistMeta.Name)
				break
			}
			trackDataIndex := sortedPlaylistTracks[i]
			trackData := cache.TrackDatas[trackDataIndex]
			fmt.Printf("   *%s\n", trackData.Name)
			//fmt.Printf("%s --- %s --- %s\n", playlistData.Name, trackData.DateTime.String(), trackData.Name)
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
	
	chunkLength := 0
	trackIndex := 0
	
	for trackIndex < totalTracks {
		chunkLength += SQUE_SPOTIFY_LIMIT_PLAYLISTS
		
		if chunkLength > totalTracks {
			chunkLength = totalTracks
		}
		
		// TODO: reuse chunk data
		trackchunk := make([]spotify.ID, chunkLength)
		for trackDataID, trackDataIndex := range(tracks[trackIndex:chunkLength]) {
			fmt.Printf("Sending %s\n", cache.TrackDatas[trackDataID].URI)

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