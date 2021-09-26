// https://github.com/zmb3/spotify/blob/master/track.go

package main

import (
	"context"
	"fmt"
	"log"
	"io/ioutil"
	"time"
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

func InitCache(c *Cache) {
	c.TrackDatasMap    = make(map[string]int)
	c.AlbumDatasMap    = make(map[string]int)
	c.PlaylistDatasMap = make(map[string]int)
	c.ArtistDatasMap   = make(map[string]int)
}

func InitConfigData(c *ConfigData, lastRunFilePath string) {
	lastRunArtists, lastRunPlaylists := parseLastRunFile(lastRunFilePath)

	fmt.Println(lastRunArtists)
	fmt.Println(lastRunPlaylists)

	// Artists
	dateTime, timeErr := time.Parse(SQUE_DATE_FORMAT, lastRunArtists)
	if timeErr != nil {
		log.Fatal(timeErr)
	}
	c.Session.LastRunArtists = dateTime
	fmt.Println(c.Session.LastRunArtists)

	// Playlists
	dateTime, timeErr = time.Parse(SQUE_DATE_FORMAT, lastRunPlaylists)
	if timeErr != nil {
		log.Fatal(timeErr)
	}
	c.Session.LastRunPlaylists = dateTime
	fmt.Println(c.Session.LastRunArtists)
	fmt.Println(c.Session.LastRunPlaylists)

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

func ScanArtistTracks(client *spotify.Client, cache *Cache, config *ConfigData, adder *TrackAdder) {
	fmt.Println("Scanning Artists....")

	dbgCount := 0

	albumType := []spotify.AlbumType{spotify.AlbumTypeSingle,spotify.AlbumTypeCompilation}

	// Followed Artists
	artists, artistErr := client.CurrentUsersFollowedArtists(context.Background(), spotify.Limit(SQUE_SPOTIFY_LIMIT_ARTISTS))
	
	continueScanning := (artistErr == nil)

	for len(artists.Artists) > 0 && continueScanning {
		fmt.Println("Artist Limit:", artists.Limit)
		fmt.Println("Number of artists:", len(artists.Artists))
		for _, artist := range artists.Artists {
			artistDataIndex, ok := cache.ArtistDatasMap[artist.ID.String()]
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

			artistAlbums, albumsErr := client.GetArtistAlbums(context.Background(), spotify.ID(artistData.ID), albumType, spotify.Limit(SQUE_SPOTIFY_LIMIT_ALBUMS))
			for len(artistAlbums.Albums) > 0 && albumsErr == nil {
				if albumsErr != nil {
					log.Fatal(albumsErr)
				}
				for _, album := range artistAlbums.Albums {

					/*
					   Some 'Compilation' spotify albums will be marked as compilation
                       even though we really want them in listen later playlist. But 
                       some compilations are actual compilations of many artists. So if
                       this album has a bunch of artists, its most likely a compilation.
                       This will probably skip cool older songs tho :'(
					*/
					if album.AlbumGroup == "appears_on" {
						continue
					}

					albumReleaseDateTime := album.ReleaseDateTime()
					//fmt.Printf("Album: %s, LastRun: %s, Current: %s\n", albumReleaseDateTime.String(), config.Session.LastRunArtists.String(), config.Session.CurrentDateTime.String())

					if albumReleaseDateTime.Before(config.Session.LastRunArtists) || albumReleaseDateTime.After(config.Session.CurrentDateTime) {
						continue
					}

					albumDataIndex, ok := cache.AlbumDatasMap[album.ID.String()]
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

					albumTracks, tracksErr := client.GetAlbumTracks(context.Background(), album.ID, spotify.Limit(SQUE_SPOTIFY_LIMIT_TRACKS), spotify.Market(SQUE_SPOTIFY_MARKET))
					
					for len(albumTracks.Tracks) > 0 && tracksErr == nil {
						
						if tracksErr != nil {
							log.Fatal(tracksErr)
						}
						
						for _, track := range albumTracks.Tracks {
							// Skip tracks that are 'intro' tracks that dont really have much music content
                    		// 80s = 80000ms
							//if track.Duration <= 80000 {
							//	continue
							//}
							trackDataIndex, ok := cache.TrackDatasMap[track.ID.String()]
							
							// if we already have the track then skip
							if ok { 
								continue
							}

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
							trackData := cache.TrackDatas[trackDataIndex]

							adder.ListenLater = append(adder.ListenLater, trackDataIndex)

							fmt.Printf("  *%s\n", trackData.Name)
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
	/*if artistErr != nil {
		log.Fatal(artistErr)
	}*/
}

func ScanPlaylistTracks(client *spotify.Client, cache *Cache, config *ConfigData, adder *TrackAdder) {
	return
	fmt.Println("Scanning Playlists....")
	
	dbgCount := 0

	// TODO: Playlist track sorting

	for playlistMetaIndex, playlistMeta := range(config.Playlists) {

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

		//fmt.Printf("why why %d, %s\n", len(playlistTracks.Tracks), playlistErr)

		for len(playlistTracks.Tracks) > 0 && scanPlaylist {

			for _, playlistTrack := range(playlistTracks.Tracks) {
				
				//fmt.Printf("%d\n", index)

				trackReleaseDateTime, dateTimeErr := time.Parse(spotify.TimestampLayout, playlistTrack.AddedAt)

				if dateTimeErr != nil {
					fmt.Printf("Cannot determine date for playlist track %s\n", playlistTrack.Track.Name)
					continue
				}

				//fmt.Printf("TrackRelease: %s, LastPlay: %s\n", trackReleaseDateTime.String(), config.Session.LastRunPlaylists.String())

				if trackReleaseDateTime.Before(config.Session.LastRunPlaylists) {
					continue
				}

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
							Score: 0,
							DateTime: trackReleaseDateTime,
					})
				}

				playlistData.Tracks = append(playlistData.Tracks, trackDataIndex)
				trackData := cache.TrackDatas[trackDataIndex]

				adder.ListenLater = append(adder.ListenLater, trackDataIndex)

				fmt.Printf("   *%s\n", trackData.Name)
				//fmt.Printf("%s --- %s --- %s\n", playlistData.Name, trackData.DateTime.String(), trackData.Name)

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