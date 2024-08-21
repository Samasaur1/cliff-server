package main

import (
    "encoding/gob"
    "flag"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "io"

    "github.com/sideshow/apns2"
    "github.com/sideshow/apns2/payload"
    "github.com/sideshow/apns2/token"
    "tailscale.com/tailcfg"
    "tailscale.com/tsnet"
)

var (
    hostname = flag.String("hostname", "cliff", "The hostname to use on the tailnet")
    apnsKey  = flag.String("apns-key", os.Getenv("CLIFF_APNS_KEY_PATH"), "Path to the APNs token signing key")
    keyID    = flag.String("key-id", os.Getenv("CLIFF_APNS_KEY_ID"), "ID of the APNs token signing key")
    teamID   = flag.String("team-id", os.Getenv("CLIFF_APNS_TEAM_ID"), "ID of the team signing the app")
    bundleID   = flag.String("bundle-id", os.Getenv("CLIFF_APP_BUNDLE_ID"), "Bundle ID of the app receiving notifications")
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
    if *bundleID == "" {
        log.Fatal("Must provide the bundle ID of the app recieving notifications (can use the CLIFF_APP_BUNDLE_ID env var)")
    }

    // MARK: - APNs client setup

    authKey, err := token.AuthKeyFromFile(*apnsKey)
    if err != nil {
        log.Fatal("Token key error:", err)
    }

    token := &token.Token{
        AuthKey: authKey,
        KeyID:   *keyID,
        TeamID:  *teamID,
    }
    client := apns2.NewTokenClient(token)

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

    // MARK: - device data setup

    type DeviceData struct {
        NodeNameAtRegistration string
        ApnsToken              string
    }
    type UserData struct {
        UsernameAtRegistration string
        Devices                map[tailcfg.StableNodeID]DeviceData
    }
    var devices map[tailcfg.UserID]UserData

    file, err := os.Open("devices.gob")
    if err == nil {
        decoder := gob.NewDecoder(file)
        err := decoder.Decode(&devices)

        if err != nil {
            devices = map[tailcfg.UserID]UserData{}
        }

        file.Close()
    } else {
        devices = map[tailcfg.UserID]UserData{}
    }

    interruptChannel := make(chan os.Signal, 1)
    signal.Notify(interruptChannel, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-interruptChannel

        file, err := os.Create("devices.gob")
        if err != nil {
            log.Printf("Unable to create file! err: %s", err.Error())
        }

        encoder := gob.NewEncoder(file)
        encoder.Encode(devices)

        file.Close()

        os.Exit(0)
    }()

    // MARK: - route setup

    mux := http.NewServeMux()

    mux.HandleFunc("POST /send", func(w http.ResponseWriter, r *http.Request) {
        // Send notification to APNs
        who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
        log.Printf("Request to send notification from user %s", who.UserProfile.LoginName)

        var responses []apns2.Response
        for _, deviceData := range devices[who.UserProfile.ID].Devices {
            notification := &apns2.Notification{
                DeviceToken: deviceData.ApnsToken,
                Topic:       *bundleID,
                Payload:     payload.NewPayload().AlertTitle("Title").AlertSubtitle("Subtitle").AlertBody("Body").Sound("default").InterruptionLevel(payload.InterruptionLevelTimeSensitive),
            }

            res, err := client.Push(notification)
            if err != nil {
                http.Error(w, err.Error(), 500)
                return
            }
            responses = append(responses, *res)
            log.Printf("Sending notification to %s", deviceData.NodeNameAtRegistration)
        }
        for _, res := range responses {
            if !res.Sent() {
                log.Printf("Unable to send notification because %s", res.Reason)
            }
        }
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

        bytes, err := io.ReadAll(io.Reader(r.Body))
        if err != nil {
            log.Printf("Unable to extract APNs token from request body")
            http.Error(w, err.Error(), 400)
        }
        apnsToken := string(bytes)

        log.Printf("APNs token: '%s'", apnsToken)

        if _, ok := devices[who.UserProfile.ID]; !ok {
            // First device for this user
            devices[who.UserProfile.ID] = UserData{
                UsernameAtRegistration: who.UserProfile.LoginName,
                Devices: map[tailcfg.StableNodeID]DeviceData{
                    who.Node.StableID: DeviceData{
                        NodeNameAtRegistration: who.Node.DisplayName(false),
                        ApnsToken:              apnsToken,
                    },
                },
            }
        } else {
            // would like to do this but i would need to replace the whole struct. not worth it
            // devices[who.UserProfile.ID].usernameAtRegistration = who.UserProfile.LoginName
            devices[who.UserProfile.ID].Devices[who.Node.StableID] = DeviceData{
                NodeNameAtRegistration: who.Node.DisplayName(false),
                ApnsToken:              apnsToken,
            }
        }
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
