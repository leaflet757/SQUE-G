package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
	
	"github.com/zmb3/spotify/v2"
	"github.com/zmb3/spotify/v2/auth"
)

var (
	auth   *spotifyauth.Authenticator
	config  ConfigData
	cache   Cache
	adder   TrackAdder
	logger  Logger

	ch       = make(chan *spotify.Client)
	appState = "abc123" // TODO: What should this be?
)

// ---------------------------------------------------------
// ---------------------------------------------------------
func main() {
	// index 0 is program name
	// index 1 is user.data
	args := os.Args

	if len(args) < 2 {
		log.Fatal("Not enough arguments were provided. Exiting early.\nPlease provide the absolute path to user.data.")
	}

	// Setup last run and playlist meta data
	InitConfigData(&config, args[1])

	// Load Options
	for i := 1; i < len(args); i++ {
		CheckOption(&config, args, i)
	}

	// ClientID, SecretID
	auth = spotifyauth.New(spotifyauth.WithClientID(config.User.ClientID), 
						   spotifyauth.WithClientSecret(config.User.ClientSecret),
						   spotifyauth.WithRedirectURL(config.User.RedirectURI), 
						   spotifyauth.WithScopes(spotifyauth.ScopePlaylistModifyPublic, spotifyauth.ScopePlaylistModifyPrivate, spotifyauth.ScopePlaylistReadPrivate, spotifyauth.ScopeUserFollowRead))

	// first start an HTTP server
	http.HandleFunc("/callback", completeAuth)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Got request for:", r.URL.String())
	})
	go func() {
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Fatal(err)
		}
	}()

	url := auth.AuthURL(appState)
	fmt.Println("Please log in to Spotify by visiting the following page in your browser:", url)

	// wait for auth to complete
	client := <-ch

	
	// use the client to make calls that require authorization
	spotifyUser, userErr := client.CurrentUser(context.Background())
	if userErr != nil {
		log.Fatal(userErr)
	}
	
	// assign user ID
	config.User.UserID = spotifyUser.ID
	
	fmt.Println("You are logged in as:", spotifyUser.ID)
	
	// Print Followed Playlists
	if (config.Session.Flags & SessionFlags_PrintFollowedPlaylists) != 0 {
		fmt.Println("----------------------------------------------")
		fmt.Println("Displaying followed playlists, exitting early.")
		fmt.Println("----------------------------------------------")
		ShowFollowedPlaylists(client, &config)
		return
	}
	
	InitCache(&cache)

	// Start Clock
	connectedStartTime := time.Now()

	// Scan Artists
	if (config.Session.Flags & SessionFlags_ScanArtists) != 0 {
		ScanArtistTracks(client, &cache, &config, &adder)
	}

	// Scan Playlists
	if (config.Session.Flags & SessionFlags_ScanPlaylists) != 0 {
		ScanPlaylistTracks(client, &cache, &config, &adder)
	}

	fmt.Printf("Adder will add %d listen later\n", len(adder.ListenLater))
	fmt.Printf("Adder will add %d sets\n", len(adder.Sets))
	fmt.Printf("Adder will add %d Compilations\n", len(adder.Compilations))
	
	// Add songs to playlists
	if len(adder.ListenLater) > 0 {
		AddTracksToPlaylist(client, &cache, config.User.PlaylistListenLater, adder.ListenLater)
	}

	if len(adder.Sets) > 0 {
		AddTracksToPlaylist(client, &cache, config.User.PlaylistSets, adder.Sets)
	}

	if len(adder.Compilations) > 0 {
		AddTracksToPlaylist(client, &cache, config.User.PlaylistCompilation, adder.Compilations)
	}

	// Print Logs
	if len(logger.ArtistMessages) > 0 || len(logger.PlaylistMessages) > 0 {
		WriteLogs(&logger, &config)
	}

	if (config.Session.Flags & SessionFlags_ScanPlaylists) != 0 {
		AlertStalePlaylistsAndSavePlaylistUpdates(&config, &cache)
	}

	CloseAndSave(&config)

	elapsedtime := time.Since(connectedStartTime)
	fmt.Printf("Done! Elapsed time to perform scan: %s", elapsedtime)
}

// ---------------------------------------------------------
// ---------------------------------------------------------
func completeAuth(w http.ResponseWriter, r *http.Request) {
	tok, err := auth.Token(r.Context(), appState, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		log.Fatal(err)
	}
	if st := r.FormValue("state"); st != appState {
		http.NotFound(w, r)
		log.Fatalf("State mismatch: %s != %s\n", st, appState)
	}

	// use the token to get an authenticated client
	client := spotify.New(auth.Client(r.Context(), tok))
	fmt.Fprintf(w, "Login Completed!")
	ch <- client
}