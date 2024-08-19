package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
	"tailscale.com/tsnet"
)

var (
    hostname = flag.String("hostname", "cliff", "The hostname to use on the tailnet")
    apnsKey = flag.String("apns-key", os.Getenv("CLIFF_APNS_KEY_PATH"), "Path to the APNs token signing key")
    keyID = flag.String("key-id", os.Getenv("CLIFF_APNS_KEY_ID"), "ID of the APNs token signing key")
    teamID = flag.String("team-id", os.Getenv("CLIFF_APNS_TEAM_ID"), "ID of the team signing the app")
)

func main() {
    flag.Parse()
    
    if *apnsKey == "" {
        log.Fatal("Must provide a path to the APNs key file (can use the CLIFF_APNS_KEY_PATH env var)")
    }
    if *keyID == "" {
        log.Fatal("Must provide the ID of the APNs key (can use the CLIFF_APNS_KEY_ID env var)")
    }
    if *teamID == "" {
        log.Fatal("Must provide the ID of the team signing the app (can use the CLIFF_APNS_TEAM_ID env var)")
    }

    // MARK: - APNs client setup

    authKey, err := token.AuthKeyFromFile(*apnsKey)
    if err != nil {
        log.Fatal("Token key error:", err)
    }

    token := &token.Token{
        AuthKey: authKey,
        KeyID: *keyID,
        TeamID: *teamID,
    }
    client := apns2.NewTokenClient(token)

    _ = client
    // notification := &apns2.Notification{}
    //
    // res, err := client.Push(notification)

    // MARK: - Tailscale setup

    s := new(tsnet.Server)
    s.Hostname = *hostname
    defer s.Close()

    listener, err := s.Listen("tcp", ":80")
    if err != nil {
        log.Fatal(err)
    }
    defer listener.Close()

    lc, err := s.LocalClient()
    if err != nil {
        log.Fatal(err)
    }

    // MARK: - route setup

    mux := http.NewServeMux()

    mux.HandleFunc("POST /send", func(w http.ResponseWriter, r *http.Request) {
        // Send notification to APNs
        who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
        if err != nil {
            http.Error(w, err.Error(), 500)
        }
        log.Printf("Request to send notification from user %s", who.UserProfile.LoginName)
    })

    mux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
        http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
        return
    })

    mux.HandleFunc("POST /register", func(w http.ResponseWriter, r *http.Request) {
        // Register this device with this Tailscale user
        who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
        log.Printf("Registering device %s for user %s", who.Node.DisplayName(false), who.UserProfile.LoginName)
    })

    mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
        http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
        return
    })

    // TODO: Potential future endpoints to eliminate notifications when viewed on other devices
    // https://stackoverflow.com/questions/34549453/how-to-sync-push-notifications-across-multiple-ios-devices

    // MARK: - run

    log.Fatal(http.Serve(listener, mux))
}
