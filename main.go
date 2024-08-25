package main

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
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

	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
)

var (
	hostname    = flag.String("hostname", "cliff", "The hostname to use on the tailnet")
	apnsKey     = flag.String("apns-key", os.Getenv("CLIFF_APNS_KEY_PATH"), "Path to the APNs token signing key")
	keyID       = flag.String("key-id", os.Getenv("CLIFF_APNS_KEY_ID"), "ID of the APNs token signing key")
	teamID      = flag.String("team-id", os.Getenv("CLIFF_APNS_TEAM_ID"), "ID of the team signing the app")
	bundleID    = flag.String("bundle-id", os.Getenv("CLIFF_APP_BUNDLE_ID"), "Bundle ID of the app receiving notifications")
	development = flag.Bool("development", false, "Whether to send APNs notifications to the dev environment")
)

func main() {
	flag.Parse()

	if *apnsKey == "" {
		flag.PrintDefaults()
		log.Fatal("Must provide a path to the APNs key file (can use the CLIFF_APNS_KEY_PATH env var)")
	}
	if *keyID == "" {
		flag.PrintDefaults()
		log.Fatal("Must provide the ID of the APNs key (can use the CLIFF_APNS_KEY_ID env var)")
	}
	if *teamID == "" {
		flag.PrintDefaults()
		log.Fatal("Must provide the ID of the team signing the app (can use the CLIFF_APNS_TEAM_ID env var)")
	}
	if *bundleID == "" {
		flag.PrintDefaults()
		log.Fatal("Must provide the bundle ID of the app recieving notifications (can use the CLIFF_APP_BUNDLE_ID env var)")
	}

	// MARK: - APNs client setup
	log.Printf("[1/6] Creating APNs client")

	authKey, err := token.AuthKeyFromFile(*apnsKey)
	if err != nil {
		log.Fatal("Token key error:", err)
	}

	token := &token.Token{
		AuthKey: authKey,
		KeyID:   *keyID,
		TeamID:  *teamID,
	}
	apnsClient := apns2.NewTokenClient(token)
	if *development {
		apnsClient.Development() // default for now, but setting in case the default changes
	} else {
		apnsClient.Production()
	}

	log.Printf("[2/6] Creating FCM client")

	app, err := firebase.NewApp(context.Background(), nil)
	if err != nil {
		log.Fatal("Unable to create Firebase app:", err)
	}
	fcmClient, err := app.Messaging(context.Background())
	if err != nil {
		log.Fatal("Unable to create FCM client")
	}

	// MARK: - Tailscale setup
	log.Printf("[3/6] Connecting to Tailscale")

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
	log.Printf("[4/6] Loading registered devices")

	type DeviceData struct {
		NodeNameAtRegistration string
		ApnsToken              string
	}
	type FcmDeviceData struct {
		NodeNameAtRegistration string
		FcmToken               string
	}
	type UserData struct {
		UsernameAtRegistration string
		Devices                map[tailcfg.StableNodeID]DeviceData
		FcmDevices             map[tailcfg.StableNodeID]FcmDeviceData
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

	for _, userData := range devices {
		log.Printf("Loaded user %s", userData.UsernameAtRegistration)

		// These nil checks don't appear to work. Whatever
		if userData.Devices == nil {
			userData.Devices = map[tailcfg.StableNodeID]DeviceData{}
		}
		for _, deviceData := range userData.Devices {
			log.Printf("..loaded device %s for user %s", deviceData.NodeNameAtRegistration, userData.UsernameAtRegistration)
		}
		if userData.FcmDevices == nil {
			userData.FcmDevices = map[tailcfg.StableNodeID]FcmDeviceData{}
		}
		for _, fcmDeviceData := range userData.FcmDevices {
			log.Printf("..loaded FCM device %s for user %s", fcmDeviceData.NodeNameAtRegistration, userData.UsernameAtRegistration)
		}
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
	log.Printf("[5/6] Creating routes")

	type NotificationContent struct {
		Title    string `json:"title"`
		Subtitle string `json:"subtitle"`
		Body     string `json:"body"`
	}

	sendNotification := func(w http.ResponseWriter, uid tailcfg.UserID, nc NotificationContent) {
		apnsPayload := payload.NewPayload()
		fcmNotification := messaging.Notification{}
		if nc.Title != "" {
			apnsPayload.AlertTitle(nc.Title)
			fcmNotification.Title = nc.Title
		}
		if nc.Subtitle != "" {
			apnsPayload.AlertSubtitle(nc.Subtitle)
		}
		if nc.Body != "" {
			apnsPayload.AlertBody(nc.Body)
			fcmNotification.Body = nc.Body
		}
		apnsPayload.Sound("default").InterruptionLevel(payload.InterruptionLevelTimeSensitive)

		// Send to all APNs devices
		for _, deviceData := range devices[uid].Devices {
			notification := &apns2.Notification{
				DeviceToken: deviceData.ApnsToken,
				Topic:       *bundleID,
				Payload:     apnsPayload,
			}

			log.Printf("..sending APNS notification to %s", deviceData.NodeNameAtRegistration)
			res, err := apnsClient.Push(notification)
			if err != nil {
				http.Error(w, err.Error(), 500)
				log.Printf("....unrecoverable error: %s", err.Error())
				return
			}
			if !res.Sent() {
				log.Printf("....unable to send notification because %s", res.Reason)
				// TODO: return error code if all notifications fail?
			}
		}
		// Send to all FCM devices
		for _, fcmDeviceData := range devices[uid].FcmDevices {
			log.Printf("..sending FCM notification to %s", fcmDeviceData.NodeNameAtRegistration)

			message := &messaging.Message{
				Notification: &fcmNotification,
				Android: &messaging.AndroidConfig{
					Priority: "high",
				},
				Token: fcmDeviceData.FcmToken,
			}
			_, err := fcmClient.Send(context.Background(), message)
			if err != nil {
				http.Error(w, err.Error(), 500)
				log.Printf("....error: %s", err.Error())
				return
			}
		}
	}

	mux := http.NewServeMux()

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
				FcmDevices: map[tailcfg.StableNodeID]FcmDeviceData{},
			}
		} else {
			if devices[who.UserProfile.ID].Devices == nil {
				devs := map[tailcfg.StableNodeID]DeviceData{
					who.Node.StableID: DeviceData{
						NodeNameAtRegistration: who.Node.DisplayName(false),
						ApnsToken:              apnsToken,
					},
				}
				devices[who.UserProfile.ID] = UserData{
					UsernameAtRegistration: who.UserProfile.LoginName,
					Devices:                devs,
					FcmDevices:             devices[who.UserProfile.ID].FcmDevices,
				}
			} else {
				devices[who.UserProfile.ID].Devices[who.Node.StableID] = DeviceData{
					NodeNameAtRegistration: who.Node.DisplayName(false),
					ApnsToken:              apnsToken,
				}
			}
		}
	})

	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	})

	mux.HandleFunc("/registerFCM", func(w http.ResponseWriter, r *http.Request) {
		// Register this device with this Tailscale user
		who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		log.Printf("Registering FCM device %s for user %s", who.Node.DisplayName(false), who.UserProfile.LoginName)

		bytes, err := io.ReadAll(io.Reader(r.Body))
		if err != nil {
			log.Printf("Unable to extract FCM token from request body")
			http.Error(w, err.Error(), 400)
		}
		fcmToken := string(bytes)

		log.Printf("FCM token: '%s'", fcmToken)

		if _, ok := devices[who.UserProfile.ID]; !ok {
			// First device for this user
			devices[who.UserProfile.ID] = UserData{
				UsernameAtRegistration: who.UserProfile.LoginName,
				Devices:                map[tailcfg.StableNodeID]DeviceData{},
				FcmDevices: map[tailcfg.StableNodeID]FcmDeviceData{
					who.Node.StableID: FcmDeviceData{
						NodeNameAtRegistration: who.Node.DisplayName(false),
						FcmToken:               fcmToken,
					},
				},
			}
		} else {
			if devices[who.UserProfile.ID].FcmDevices == nil {
				devs := map[tailcfg.StableNodeID]FcmDeviceData{
					who.Node.StableID: FcmDeviceData{
						NodeNameAtRegistration: who.Node.DisplayName(false),
						FcmToken:               fcmToken,
					},
				}
				devices[who.UserProfile.ID] = UserData{
					UsernameAtRegistration: who.UserProfile.LoginName,
					Devices:                devices[who.UserProfile.ID].Devices,
					FcmDevices:             devs,
				}
			} else {
				devices[who.UserProfile.ID].FcmDevices[who.Node.StableID] = FcmDeviceData{
					NodeNameAtRegistration: who.Node.DisplayName(false),
					FcmToken:               fcmToken,
				}
			}
		}
	})

	mux.HandleFunc("GET /send", func(w http.ResponseWriter, r *http.Request) {
		// Send notification
		who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		log.Printf("Request to send simple notification from user %s", who.UserProfile.LoginName)

		nc := NotificationContent{
			Body: fmt.Sprintf("Notification triggered by %s", who.Node.DisplayName(false)),
		}
		sendNotification(w, who.UserProfile.ID, nc)
	})

	mux.HandleFunc("POST /send", func(w http.ResponseWriter, r *http.Request) {
		// Send notification to APNs
		who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		log.Printf("Request to send notification with data from user %s", who.UserProfile.LoginName)

		err = r.ParseForm()
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		nc := NotificationContent{
			Title:    r.Form["title"][0],
			Subtitle: r.Form["subtitle"][0],
			Body:     r.Form["body"][0],
		}

		if nc.Title == "" && nc.Body == "" {
			// This notification would have no content
			log.Printf("..notification has none of: title, body")
			http.Error(w, "Notification must have content", 400)
			return
		}

		sendNotification(w, who.UserProfile.ID, nc)
	})

	mux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	})

	mux.HandleFunc("POST /sendJSON", func(w http.ResponseWriter, r *http.Request) {
		// Send notification to APNs
		who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		log.Printf("Request to send notification with JSON from user %s", who.UserProfile.LoginName)

		var nc NotificationContent
		err = json.NewDecoder(r.Body).Decode(&nc)
		if err != nil {
			log.Printf("..invalid JSON")
			http.Error(w, err.Error(), 400)
			return
		}

		if nc.Title == "" && nc.Body == "" {
			// This notification would have no content
			log.Printf("..notification has none of: title, body")
			http.Error(w, "Notification must have content", 400)
			return
		}

		sendNotification(w, who.UserProfile.ID, nc)
	})

	mux.HandleFunc("/sendJSON", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	})

	// TODO: Potential future endpoints to eliminate notifications when viewed on other devices
	// https://stackoverflow.com/questions/34549453/how-to-sync-push-notifications-across-multiple-ios-devices

	// MARK: - run
	log.Printf("[6/6] Launching server")

	log.Fatal(http.Serve(listener, mux))
}
