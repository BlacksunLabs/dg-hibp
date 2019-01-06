package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"

	"github.com/BlacksunLabs/drgero/event"
	"github.com/BlacksunLabs/drgero/mq"
)

// Response is the full JSON response body to the HIBP API v2
type Response struct {
	Name         string   `json:"Name"`
	Title        string   `json:"Title"`
	Domain       string   `json:"Domain"`
	BreachDate   string   `json:"BreachDate"`
	AddedDate    string   `json:"AddedDate"`
	ModifiedDate string   `json:"ModifiedDate"`
	PwnCount     int      `json:"PwnCount"`
	Description  string   `json:"Description"`
	LogoPath     string   `json:"LogoPath"`
	DataClasses  []string `json:"DataClasses"`
	IsVerified   bool     `json:"IsVerified"`
	IsFabricated bool     `json:"IsFabricated"`
	IsSensitive  bool     `json:"IsSensitive"`
	IsRetired    bool     `json:"IsRetired"`
	IsSpamList   bool     `json:"IsSpamList"`
}

// Results is a collection of Responses
type Results struct {
	Entries []Response `json:"Entries"`
}
type postBody struct {
	Body Results `json:"body"`
}

var m = new(mq.Client)
var connectString string
var hostString string

func checkEmail(email string) (r *Results, err error) {
	email = url.QueryEscape(email)

	url := fmt.Sprintf("https://haveibeenpwned.com/api/v2/breachedaccount/%s?includeUnverified=true", email)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("error creating new request: %v")
		return &Results{}, err
	}

	req.Header.Add("User-Agent", "Dr.Gero")
	req.Header.Add("api-version", "2")
	req.Header.Add("cache-control", "no-cache")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return &Results{}, err
	}
	defer res.Body.Close()

	body, _ := ioutil.ReadAll(res.Body)

	r = &Results{}
	err = json.Unmarshal(body, &r.Entries)
	if err != nil {
		return &Results{}, err
	}

	return r, nil
}

func hasEmail(text string) bool {
	re := regexp.MustCompile(`mailto:.*@.*\..* ?`)

	match := re.FindStringSubmatch(text)
	if len(match) < 1 {
		// log.Printf("string does not contain email")
		return false
	}

	return true
}

func extractEmail(text string) (string, error) {

	re := regexp.MustCompile(`mailto:(.*@.*\.[a-zA-Z]+)`)

	match := re.FindStringSubmatch(text)
	if len(match) < 1 || match[1] == "" {
		return "", fmt.Errorf("no match found")
	}

	return match[1], nil
}

func post(payload []byte, url string) error {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "hibpwner")

	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		return err
	}

	return nil
}

// Send sends a JSON encoded Event a Dr.Gero host
func (r *Results) send(host string) error {
	var payload postBody
	payload.Body = *r
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/event", host)
	err = post(body, url)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	connectString = os.Getenv("DG_CONNECT")
	hostString = os.Getenv("DG_HOST")

	if connectString == "" || hostString == "" {
		log.Fatal("Must provide rabbitmq connect string and Dr.Gero API host in DG_CONNECT and DG_HOST environment variables, respectively")
		return
	}

	// err := m.Connect("amqp://guest:guest@localhost:5672")
	err := m.Connect(connectString)
	if err != nil {
		fmt.Printf("unable to connect to RabbitMQ : %v", err)
	}

	queueName, err := m.NewTempQueue()
	if err != nil {
		fmt.Printf("could not create temporary queue : %v", err)
	}

	err = m.BindQueueToExchange(queueName, "events")
	if err != nil {
		fmt.Printf("%v", err)
		return
	}

	ch, err := m.GetChannel()
	if err != nil {
		fmt.Printf("%v", err)
		return
	}

	events, err := ch.Consume(
		queueName,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		fmt.Printf("failed to register consumer to %s : %v", queueName, err)
		return
	}

	forever := make(chan bool)

	go func() {
		for e := range events {
			var event = new(event.Event)

			var err = json.Unmarshal(e.Body, event)
			if err != nil {
				fmt.Printf("failed to unmarshal event: %v", err)
				<-forever
			}

			ok := hasEmail(event.Message)
			if !ok {
				log.Printf("no email found")
				break
			}

			email, err := extractEmail(event.Message)
			if err != nil {
				log.Printf("failed to extract email from %s : %v", event.Message, err)
				break
			}

			hits, err := checkEmail(email)
			if err != nil {
				log.Printf("error with HIBP API: %v", err)
			}

			log.Printf("Sending results to Dr.Gero")
			err = hits.send(hostString)
			if err != nil {
				log.Printf("failed sending to Dr.Gero: %v", err)
			}
		}
	}()

	fmt.Println("[i] Waiting for events. To exit press CTRL+C")
	<-forever
}
