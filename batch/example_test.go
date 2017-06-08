// Copyright 2015 James Cote and Liberty Fund, Inc.
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package batch_test

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/jfcote87/google-api-go-client/batch"
	"github.com/jfcote87/google-api-go-client/batch/credentials"

	cal "google.golang.org/api/calendar/v3"
	gmail "google.golang.org/api/gmail/v1"
	storage "google.golang.org/api/storage/v1"

	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
)

type EventFromDb struct {
	Id          string `json:"id"`
	Title       string `json:"nm"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Description string `json:"desc"` // full description
	Link        string `json:"link"` // web link to full event info
	Loc         string `json:"loc"`  // Event location
}

func ExampleService_calendar() {
	var ctx context.Context = context.Background()
	var calendarId string = "xxxxxxxxxxxxx@group.calendar.google.com"
	var events []*EventFromDb = getEventData()
	var oauthClient *http.Client = getOauthClient()
	// Read through slice of EventFromDb and add to batch.  Then call Do() to send
	// and process responses
	bsv := batch.Service{Client: oauthClient}
	calsv, _ := cal.New(batch.BatchClient)

	for _, ev := range events {
		// create Event
		event := &cal.Event{
			Summary:            ev.Title,
			Description:        ev.Description,
			Location:           ev.Loc,
			Start:              &cal.EventDateTime{DateTime: ev.Start},
			End:                &cal.EventDateTime{DateTime: ev.End},
			Reminders:          &cal.EventReminders{UseDefault: false},
			Transparency:       "transparent",
			Source:             &cal.EventSource{Title: "Web Link", Url: ev.Link},
			ExtendedProperties: &cal.EventExtendedProperties{Shared: map[string]string{"DBID": ev.Id}},
		}

		event, err := calsv.Events.Insert(calendarId, event).Do()
		// queue new request in batch service
		err = bsv.AddRequest(err,
			batch.SetResult(&event),
			batch.SetTag(ev),
		)
		if err != nil {
			log.Println(err)
			return
		}
	}

	// execute batch
	responses, err := bsv.DoCtx(ctx)
	if err != nil {
		log.Println(err)
		return
	}
	for _, r := range responses {
		var event *cal.Event
		tag := r.Tag.(*EventFromDb)
		if r.Err != nil {
			log.Printf("Error adding event (Id: %s %s): %v", tag.Id, tag.Title, r.Err)
			continue
		}
		event = r.Result.(*cal.Event)
		updateDatabaseorSomething(tag, event.Id)
	}
	return
}

func updateDatabaseorSomething(ev *EventFromDb, newCalEventId string) {
	// Logic for database update or post processing
	return
}

func getEventData() []*EventFromDb {
	// do something to retrieve data from a database
	return nil
}

func getOauthClient() *http.Client {
	return nil
}

func ExampleService_userdata() {
	var ctx context.Context = context.Background()
	projectId, usernames, config := getInitialData()
	// Retrieve the list of available buckets for each user for a given api project as well as
	// profile info for each person
	bsv := batch.Service{} // no need for client as individual requests will have their own authorization
	storagesv, _ := storage.New(batch.BatchClient)
	gsv, _ := gmail.New(batch.BatchClient)
	config.Scopes = []string{gmail.MailGoogleComScope, "email", storage.DevstorageReadOnlyScope}

	for _, u := range usernames {
		// create new credentials for specific user
		tConfig := *config
		tConfig.Subject = u + "@example.com"
		cred := &credentials.Oauth2Credentials{TokenSource: tConfig.TokenSource(context.Background())}

		// create bucket list request
		bucketList, err := storagesv.Buckets.List(projectId).Do()
		if err = bsv.AddRequest(err,
			batch.SetResult(&bucketList),
			batch.SetTag([]string{u, "Buckets"}),
			batch.SetCredentials(cred)); err != nil {
			log.Printf("Error adding bucklist request for %s: %v", u, err)
			return
		}

		// create profile request
		profile, err := gsv.Users.GetProfile(u + "@example.com").Do()
		if err = bsv.AddRequest(err,
			batch.SetResult(&profile),
			batch.SetTag([]string{u, "Profile"}),
			batch.SetCredentials(cred)); err != nil {
			log.Printf("Error adding profile request for %s: %v", u, err)
			return
		}
	}

	// execute batch
	responses, err := bsv.DoCtx(ctx)
	if err != nil {
		log.Println(err)
		return
	}
	// process responses
	for _, r := range responses {
		tag := r.Tag.([]string)
		if r.Err != nil {
			log.Printf("Error retrieving user (%s) %s: %v", tag[0], tag[1], r.Err)
			continue
		}
		if tag[1] == "Profile" {
			profile := r.Result.(*gmail.Profile)
			log.Printf("User %s profile id is %d", profile.EmailAddress, profile.HistoryId)
		} else {
			blist := r.Result.(*storage.Buckets)
			log.Printf("User: %s", tag[0])
			for _, b := range blist.Items {
				log.Printf("%s", b.Name)
			}
		}
	}
	return
}

func getInitialData(scopes ...string) (string, []string, *jwt.Config) {
	jwtbytes, _ := ioutil.ReadFile("secret.json")

	config, _ := google.JWTConfigFromJSON(jwtbytes, scopes...)
	return "XXXXXXXXXXXXXXXX", []string{"jcote", "bclinton", "gbush", "bobama"}, config
}
