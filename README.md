# SQUE-G (Song Queuer Utility Engine in Go)

I don't like seeing notifications on my phone unless they're important which was the main motivation behind this script. Instead of applications sending notifications for recently released tracks whenever the application determines is appropriate, this script allows the user to scan for new tracks whenever the user chooses to actually look for new tracks.

## Example usage:
```
go run . C:/place/on/my/drive/user.data -a -p
go run . <user.data> [options]
```

## Options
- -a : scan artists
- -p : scan playlists
- -d \<date\> : overwrite last artist and/or playlist run date, \<year-month-day,year-month-day\>
- -fp : print followed playlists

Running this will open up a webbrowser window asking to allow the script access of your Spotify
account. Scroll all the way to the bottom without reading any of the TOS and click the accept
button. This will open a new page to example.com. Copy the entire URL of this page and paste
it into the terminal window.

## User Data File
The \<user.data\> file must be in JSON format and be of the form:
```
{
    "user":
    {
        "client_id":"xxxxxxxxxx",
        "client_secret":"xxxxxxxxxx",
        "redirect_uri":"http://localhost:8080/callback",

        "logs_path":"C:/path/to/custom/log/dir",
        "last_run_path":"C:/path/to/last/run/file",
        
        "listen_later":"xxxxxxxxxx",
        "compilation":"xxxxxxxxxx",
        "sets":"xxxxxxxxxx"
    },
    "playlists":
    [  
        {
            "name":"Human Music Playlist",
            "id":"xxxxxxxxxx",
            "limit":"-1"
        },
    ]
}
```

## Last Run File
The \<lastrun\> file must only contain numbers separated by dashes for each last run category (artists,playlists):
```
year-month-day,year-month-day
```

## TODO
- check for track dups, uri check done - wat else?
- clean up this shitty code
- Add a queuer for followed podcasts
