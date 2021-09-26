package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

	ch       = make(chan *spotify.Client)
	appState = "abc123"
)

// ------------------------------
// Main
func main() {
	// index 0 is program name
	// index 1 is user.data
	// index 2 is lastrun
	args := os.Args

	// Load json
	configBytes, configErr := ioutil.ReadFile(args[1])
    if configErr != nil {
		fmt.Print("error 1:", configErr)
	}
	
    // unmarshall it, copy to object
    configErr = json.Unmarshal(configBytes, &config)
    if configErr != nil {
        fmt.Println("error 2:", configErr)
    }

	InitConfigData(&config, args[2])

	// ClientID, SecretID
	auth = spotifyauth.New(spotifyauth.WithClientID(config.User.ClientID), spotifyauth.WithClientSecret(config.User.ClientSecret), spotifyauth.WithRedirectURL(config.User.RedirectURI), spotifyauth.WithScopes(spotifyauth.ScopePlaylistModifyPublic, spotifyauth.ScopePlaylistModifyPrivate, spotifyauth.ScopeUserLibraryModify, spotifyauth.ScopeUserFollowRead))

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

	connectedStartTime := time.Now()

	// use the client to make calls that require authorization
	spotifyUser, userErr := client.CurrentUser(context.Background())
	if userErr != nil {
		log.Fatal(userErr)
	}
	fmt.Println("You are logged in as:", spotifyUser.ID)

	InitCache(&cache)
	ScanArtistTracks(client, &cache, &config, &adder)
	ScanPlaylistTracks(client, &cache, &config, &adder)

	fmt.Printf("Adder will add %d listen later\n", len(adder.ListenLater))
	fmt.Printf("Adder will add %d sets\n", len(adder.Sets))
	fmt.Printf("Adder will add %d Compilations\n", len(adder.Compilations))
	
	if len(adder.ListenLater) > 0 {
		AddTracksToPlaylist(client, &cache, config.User.PlaylistListenLater, adder.ListenLater)
	}

	if len(adder.Sets) > 0 {
		AddTracksToPlaylist(client, &cache, config.User.PlaylistSets, adder.Sets)
	}

	if len(adder.Compilations) > 0 {
		AddTracksToPlaylist(client, &cache, config.User.PlaylistCompilation, adder.Compilations)
	}

	elapsedtime := time.Since(connectedStartTime)
	fmt.Printf("Elapsed time to perform scan: %s", elapsedtime)
}

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