package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Credential struct {
	token  string
	expiry int64 // Timestamp in Unix time (comparable against time.Now().Unix()) when the token will expire
}

var credentials map[string]Credential

func main() {
	credentials = make(map[string]Credential)
	http.HandleFunc("/federate", proxyRequest)
	log.Println("OpenShift Federation Proxy will now handle requests.")
	http.ListenAndServe(":8080", nil)
}

func proxyRequest(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	endpoint := query.Get("endpoint")
	username := query.Get("username")
	password := query.Get("password")
	token := query.Get("token")

	if len(endpoint) == 0 || ((len(username) == 0 || len(password) == 0) && len(token) == 0) {
		log.Println("URL parameters are not set correctly.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	authEndpoint := fmt.Sprintf("https://oauth-openshift.apps.%s/oauth/authorize?client_id=openshift-challenging-client&response_type=token", endpoint)
	prometheusEndpoint := fmt.Sprintf("https://prometheus-k8s-openshift-monitoring.apps.%s/federate", endpoint)

	// Authenticate (if needed)

	if len(token) > 0 {
		// Use provided token
		credentials[authEndpoint] = Credential{
			token: token,
			expiry: 86400, // Assume token is good for one day (it's probably going to be refreshed before then anyway)
		}
	} else {
		// Auth using username & password
		credential, found := credentials[authEndpoint]
		if !found || credential.expiry < time.Now().Unix() {
			fmt.Printf("Fetching new token for %s\n", endpoint)
			err := refreshToken(authEndpoint, username, password)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
	}

	// Proxy the request

	req, err := http.NewRequest("GET", prometheusEndpoint, nil)
	if err != nil {
		return
	}
	query.Del("username")
	query.Del("password")
	query.Del("endpoint")
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Authorization", "Bearer "+credentials[authEndpoint].token)
	client := &http.Client{}
	resp, err := client.Do(req)

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error fetching:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if strings.Contains(string(bodyBytes), "<title>Log In</title>") {
		log.Printf("Credentials for %s have stopped working unexpectedly - invalidating them. Please retry request.\n", endpoint)
		delete(credentials, authEndpoint)
		w.WriteHeader(http.StatusServiceUnavailable) // indicating please retry later
		return
	}

	w.Write(bodyBytes)
}

func refreshToken(endpoint string, username string, password string) error {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return err
	}

	req.SetBasicAuth(username, password)

	client := &http.Client{}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// We have to throw an error here, or the client will follow the redirect (which we don't want)
		return errors.New("Redirect (This is expected)")
	}

	resp, err := client.Do(req)
	if err != nil && resp.StatusCode != http.StatusFound {
		// Only throw a real error if the code isn't the redirect that we expected
		return err
	}

	locationHeader, err := resp.Location()

	var accessTokenRegex = regexp.MustCompile("access_token=([^&]*)")
	accessToken := accessTokenRegex.FindStringSubmatch(locationHeader.String())
	if len(accessToken) < 2 {
		return errors.New("Could not parse token from OAuth proxy")
	}

	var expiryRegex = regexp.MustCompile("expires_in=([^&]*)")
	expiry := expiryRegex.FindStringSubmatch(locationHeader.String())
	if len(expiry) < 2 {
		return errors.New("Could not parse expiry from OAuth proxy")
	}
	expiryInt, err := strconv.ParseInt(expiry[1], 10, 64)
	if err != nil {
		return errors.New("Could not parse expiry from OAuth proxy")
	}

	credentials[endpoint] = Credential{
		token:  accessToken[1],
		expiry: time.Now().Unix() + expiryInt - 5,
	}

	return nil
}
